package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"cargo-transport-service/core/internal/auth"

	pb "cargo-transport-service/core/gen/cargo/v1"
)

const userCols = `id::text, name, COALESCE(email, ''), COALESCE(phone, ''),
	COALESCE(organization_id::text, ''), created_at`

var phoneRe = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

type AuthService struct {
	pb.UnimplementedAuthServiceServer
	pool *pgxpool.Pool
	jwt  *auth.Manager
}

func NewAuthService(pool *pgxpool.Pool, jwt *auth.Manager) *AuthService {
	return &AuthService{pool: pool, jwt: jwt}
}

func (s *AuthService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if len(req.Password) < 6 {
		return nil, status.Error(codes.InvalidArgument, "password must be at least 6 characters")
	}
	email, phone, err := normalizeContact(req.Email, req.Phone)
	if err != nil {
		return nil, err
	}
	if email == "" && phone == "" {
		return nil, status.Error(codes.InvalidArgument, "email or phone is required")
	}
	if req.OrganizationId != "" {
		if err := checkID(req.OrganizationId, "organization_id"); err != nil {
			return nil, err
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not hash password")
	}

	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (name, email, phone, password_hash, organization_id)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, '')::uuid)
		 RETURNING `+userCols,
		name, email, phone, string(hash), req.OrganizationId))
	if err != nil {
		return nil, dbErr(err)
	}
	return s.authResponse(u)
}

func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	email, phone, err := normalizeContact(req.Email, req.Phone)
	if err != nil {
		return nil, err
	}
	if (email == "") == (phone == "") {
		return nil, status.Error(codes.InvalidArgument, "provide either email or phone")
	}

	var (
		u    pb.User
		hash string
		made time.Time
	)
	err = s.pool.QueryRow(ctx,
		`SELECT `+userCols+`, password_hash FROM users
		 WHERE email = NULLIF($1, '') OR phone = NULLIF($2, '')`, email, phone).
		Scan(&u.Id, &u.Name, &u.Email, &u.Phone, &u.OrganizationId, &made, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	u.CreatedAt = ts(made)
	return s.authResponse(&u)
}

func (s *AuthService) GetMe(ctx context.Context, _ *emptypb.Empty) (*pb.User, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1::uuid`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return u, nil
}

func (s *AuthService) authResponse(u *pb.User) (*pb.AuthResponse, error) {
	token, err := s.jwt.Issue(u.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not sign token")
	}
	return &pb.AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.jwt.TTL().Seconds()),
		User:        u,
	}, nil
}

func scanUser(row pgx.Row) (*pb.User, error) {
	var (
		u    pb.User
		made time.Time
	)
	if err := row.Scan(&u.Id, &u.Name, &u.Email, &u.Phone, &u.OrganizationId, &made); err != nil {
		return nil, err
	}
	u.CreatedAt = ts(made)
	return &u, nil
}

func normalizeContact(email, phone string) (string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	phone = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(phone))

	if email != "" && !strings.Contains(email, "@") {
		return "", "", status.Error(codes.InvalidArgument, "email is not valid")
	}
	if phone != "" && !phoneRe.MatchString(phone) {
		return "", "", status.Error(codes.InvalidArgument, "phone must be 7-15 digits, optionally starting with +")
	}
	return email, phone, nil
}
