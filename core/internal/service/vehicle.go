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

const vehicleCols = `id::text, owner_id::text, plate_number, model, capacity_kg, created_at`

type VehicleService struct {
	pb.UnimplementedVehicleServiceServer
	pool *pgxpool.Pool
}

func NewVehicleService(pool *pgxpool.Pool) *VehicleService {
	return &VehicleService{pool: pool}
}

func (s *VehicleService) CreateVehicle(ctx context.Context, req *pb.CreateVehicleRequest) (*pb.Vehicle, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	plate, err := validateVehicle(req.PlateNumber, req.CapacityKg)
	if err != nil {
		return nil, err
	}

	v, err := scanVehicle(s.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_id, plate_number, model, capacity_kg)
		 VALUES ($1::uuid, $2, $3, $4) RETURNING `+vehicleCols,
		userID, plate, strings.TrimSpace(req.Model), req.CapacityKg))
	if err != nil {
		return nil, dbErr(err)
	}
	return v, nil
}

func (s *VehicleService) GetVehicle(ctx context.Context, req *pb.GetVehicleRequest) (*pb.Vehicle, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}

	v, err := scanVehicle(s.pool.QueryRow(ctx,
		`SELECT `+vehicleCols+` FROM vehicles WHERE id = $1::uuid AND owner_id = $2::uuid`, req.Id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "vehicle not found")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return v, nil
}

func (s *VehicleService) ListVehicles(ctx context.Context, _ *emptypb.Empty) (*pb.ListVehiclesResponse, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+vehicleCols+` FROM vehicles WHERE owner_id = $1::uuid ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()

	resp := &pb.ListVehiclesResponse{}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		resp.Vehicles = append(resp.Vehicles, v)
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return resp, nil
}

func (s *VehicleService) UpdateVehicle(ctx context.Context, req *pb.UpdateVehicleRequest) (*pb.Vehicle, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	plate, err := validateVehicle(req.PlateNumber, req.CapacityKg)
	if err != nil {
		return nil, err
	}

	v, err := scanVehicle(s.pool.QueryRow(ctx,
		`UPDATE vehicles SET plate_number = $3, model = $4, capacity_kg = $5
		 WHERE id = $1::uuid AND owner_id = $2::uuid RETURNING `+vehicleCols,
		req.Id, userID, plate, strings.TrimSpace(req.Model), req.CapacityKg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "vehicle not found")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return v, nil
}

func (s *VehicleService) DeleteVehicle(ctx context.Context, req *pb.DeleteVehicleRequest) (*emptypb.Empty, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM vehicles WHERE id = $1::uuid AND owner_id = $2::uuid`, req.Id, userID)
	if err != nil {
		return nil, dbErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, status.Error(codes.NotFound, "vehicle not found")
	}
	return &emptypb.Empty{}, nil
}

func validateVehicle(plate string, capacityKg int32) (string, error) {
	plate = strings.ToUpper(strings.TrimSpace(plate))
	if plate == "" {
		return "", status.Error(codes.InvalidArgument, "plate_number is required")
	}
	if capacityKg <= 0 {
		return "", status.Error(codes.InvalidArgument, "capacity_kg must be positive")
	}
	return plate, nil
}

func scanVehicle(row pgx.Row) (*pb.Vehicle, error) {
	var (
		v    pb.Vehicle
		made time.Time
	)
	if err := row.Scan(&v.Id, &v.OwnerId, &v.PlateNumber, &v.Model, &v.CapacityKg, &made); err != nil {
		return nil, err
	}
	v.CreatedAt = ts(made)
	return &v, nil
}
