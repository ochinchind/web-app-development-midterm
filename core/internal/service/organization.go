package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"cargo-transport-service/core/internal/auth"

	pb "cargo-transport-service/core/gen/cargo/v1"
)

const orgCols = `id::text, name, created_at`

type OrganizationService struct {
	pb.UnimplementedOrganizationServiceServer
	pool *pgxpool.Pool
}

func NewOrganizationService(pool *pgxpool.Pool) *OrganizationService {
	return &OrganizationService{pool: pool}
}

func (s *OrganizationService) CreateOrganization(ctx context.Context, req *pb.CreateOrganizationRequest) (*pb.Organization, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, dbErr(err)
	}
	defer tx.Rollback(ctx)

	org, err := scanOrg(tx.QueryRow(ctx, `INSERT INTO organizations (name) VALUES ($1) RETURNING `+orgCols, name))
	if err != nil {
		return nil, dbErr(err)
	}
	// The creator becomes a member, unless they already belong to one.
	if _, err := tx.Exec(ctx,
		`UPDATE users SET organization_id = $1::uuid WHERE id = $2::uuid AND organization_id IS NULL`,
		org.Id, userID); err != nil {
		return nil, dbErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, dbErr(err)
	}
	return org, nil
}

func (s *OrganizationService) GetOrganization(ctx context.Context, req *pb.GetOrganizationRequest) (*pb.Organization, error) {
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	org, err := scanOrg(s.pool.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE id = $1::uuid`, req.Id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "organization not found")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return org, nil
}

func (s *OrganizationService) ListOrganizations(ctx context.Context, _ *emptypb.Empty) (*pb.ListOrganizationsResponse, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+orgCols+` FROM organizations ORDER BY created_at DESC`)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()

	resp := &pb.ListOrganizationsResponse{}
	for rows.Next() {
		org, err := scanOrg(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		resp.Organizations = append(resp.Organizations, org)
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return resp, nil
}

func (s *OrganizationService) UpdateOrganization(ctx context.Context, req *pb.UpdateOrganizationRequest) (*pb.Organization, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	org, err := scanOrg(s.pool.QueryRow(ctx,
		`UPDATE organizations SET name = $2
		 WHERE id = $1::uuid
		   AND EXISTS (SELECT 1 FROM users WHERE id = $3::uuid AND organization_id = $1::uuid)
		 RETURNING `+orgCols, req.Id, name, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, s.explainMiss(ctx, req.Id)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return org, nil
}

func (s *OrganizationService) DeleteOrganization(ctx context.Context, req *pb.DeleteOrganizationRequest) (*emptypb.Empty, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM organizations
		 WHERE id = $1::uuid
		   AND EXISTS (SELECT 1 FROM users WHERE id = $2::uuid AND organization_id = $1::uuid)`,
		req.Id, userID)
	if err != nil {
		return nil, dbErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, s.explainMiss(ctx, req.Id)
	}
	return &emptypb.Empty{}, nil
}

func (s *OrganizationService) explainMiss(ctx context.Context, id string) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
		return dbErr(err)
	}
	if !exists {
		return status.Error(codes.NotFound, "organization not found")
	}
	return status.Error(codes.PermissionDenied, "you are not a member of this organization")
}

func scanOrg(row pgx.Row) (*pb.Organization, error) {
	var (
		o    pb.Organization
		made time.Time
	)
	if err := row.Scan(&o.Id, &o.Name, &made); err != nil {
		return nil, err
	}
	o.CreatedAt = ts(made)
	return &o, nil
}
