package service

import (
	"context"
	"errors"
	"fmt"
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

const orderCols = `id::text, created_by::text, COALESCE(organization_id::text, ''),
	from_address, to_address, cargo_description, weight_kg, status,
	COALESCE(driver_id::text, ''), COALESCE(vehicle_id::text, ''),
	created_at, updated_at, taken_at, delivered_at`

type OrderService struct {
	pb.UnimplementedOrderServiceServer
	pool *pgxpool.Pool
}

func NewOrderService(pool *pgxpool.Pool) *OrderService {
	return &OrderService{pool: pool}
}

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.Order, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	from, to, cargo, err := validateOrder(req.FromAddress, req.ToAddress, req.CargoDescription, req.WeightKg)
	if err != nil {
		return nil, err
	}

	o, err := scanOrder(s.pool.QueryRow(ctx,
		`INSERT INTO orders (created_by, organization_id, from_address, to_address, cargo_description, weight_kg)
		 VALUES ($1::uuid, (SELECT organization_id FROM users WHERE id = $1::uuid), $2, $3, $4, $5)
		 RETURNING `+orderCols,
		userID, from, to, cargo, req.WeightKg))
	if err != nil {
		return nil, dbErr(err)
	}
	return o, nil
}

func (s *OrderService) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.Order, error) {
	if _, err := auth.UserID(ctx); err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	return s.load(ctx, req.Id)
}

func (s *OrderService) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}

	where := []string{"TRUE"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if req.Status != pb.OrderStatus_UNSPECIFIED {
		where = append(where, "status = "+arg(req.Status.String()))
	}
	if req.CreatedByMe {
		where = append(where, "created_by = "+arg(userID)+"::uuid")
	}
	if req.TakenByMe {
		where = append(where, "driver_id = "+arg(userID)+"::uuid")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := max(req.Offset, 0)

	query := `SELECT ` + orderCols + ` FROM orders WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY created_at DESC LIMIT ` + arg(limit) + ` OFFSET ` + arg(offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()

	resp := &pb.ListOrdersResponse{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		resp.Orders = append(resp.Orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return resp, nil
}

func (s *OrderService) UpdateOrder(ctx context.Context, req *pb.UpdateOrderRequest) (*pb.Order, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	from, to, cargo, err := validateOrder(req.FromAddress, req.ToAddress, req.CargoDescription, req.WeightKg)
	if err != nil {
		return nil, err
	}

	o, err := scanOrder(s.pool.QueryRow(ctx,
		`UPDATE orders SET from_address = $3, to_address = $4, cargo_description = $5, weight_kg = $6, updated_at = now()
		 WHERE id = $1::uuid AND created_by = $2::uuid AND status = 'NEW'
		 RETURNING `+orderCols,
		req.Id, userID, from, to, cargo, req.WeightKg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, s.explainCreatorMiss(ctx, req.Id, userID)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return o, nil
}

func (s *OrderService) DeleteOrder(ctx context.Context, req *pb.DeleteOrderRequest) (*emptypb.Empty, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM orders WHERE id = $1::uuid AND created_by = $2::uuid AND status = 'NEW'`, req.Id, userID)
	if err != nil {
		return nil, dbErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, s.explainCreatorMiss(ctx, req.Id, userID)
	}
	return &emptypb.Empty{}, nil
}

func (s *OrderService) TakeOrder(ctx context.Context, req *pb.TakeOrderRequest) (*pb.Order, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}
	if err := checkID(req.VehicleId, "vehicle_id"); err != nil {
		return nil, err
	}

	var ownsVehicle bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM vehicles WHERE id = $1::uuid AND owner_id = $2::uuid)`,
		req.VehicleId, userID).Scan(&ownsVehicle); err != nil {
		return nil, dbErr(err)
	}
	if !ownsVehicle {
		return nil, status.Error(codes.FailedPrecondition, "you need one of your own vehicles to take an order")
	}

	o, err := scanOrder(s.pool.QueryRow(ctx,
		`UPDATE orders SET status = 'TAKEN', driver_id = $2::uuid, vehicle_id = $3::uuid, taken_at = now(), updated_at = now()
		 WHERE id = $1::uuid AND status = 'NEW' AND created_by <> $2::uuid
		 RETURNING `+orderCols,
		req.Id, userID, req.VehicleId))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := s.load(ctx, req.Id)
		if err != nil {
			return nil, err
		}
		if existing.CreatedBy == userID {
			return nil, status.Error(codes.FailedPrecondition, "you cannot take your own order")
		}
		return nil, status.Errorf(codes.FailedPrecondition, "order is already %s", existing.Status)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return o, nil
}

func (s *OrderService) DeliverOrder(ctx context.Context, req *pb.DeliverOrderRequest) (*pb.Order, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkID(req.Id, "id"); err != nil {
		return nil, err
	}

	o, err := scanOrder(s.pool.QueryRow(ctx,
		`UPDATE orders SET status = 'DELIVERED', delivered_at = now(), updated_at = now()
		 WHERE id = $1::uuid AND driver_id = $2::uuid AND status = 'TAKEN'
		 RETURNING `+orderCols,
		req.Id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := s.load(ctx, req.Id)
		if err != nil {
			return nil, err
		}
		if existing.DriverId != userID {
			return nil, status.Error(codes.PermissionDenied, "only the driver who took this order can deliver it")
		}
		return nil, status.Errorf(codes.FailedPrecondition, "order is %s, expected TAKEN", existing.Status)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return o, nil
}

func (s *OrderService) load(ctx context.Context, id string) (*pb.Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return o, nil
}

// explainCreatorMiss explains why an update/delete restricted to the creator of a NEW order matched nothing.
func (s *OrderService) explainCreatorMiss(ctx context.Context, id, userID string) error {
	existing, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if existing.CreatedBy != userID {
		return status.Error(codes.PermissionDenied, "only the creator can change this order")
	}
	return status.Errorf(codes.FailedPrecondition, "order is %s, only NEW orders can be changed", existing.Status)
}

func validateOrder(from, to, cargo string, weightKg int32) (string, string, string, error) {
	from, to, cargo = strings.TrimSpace(from), strings.TrimSpace(to), strings.TrimSpace(cargo)
	if from == "" || to == "" {
		return "", "", "", status.Error(codes.InvalidArgument, "from_address and to_address are required")
	}
	if weightKg <= 0 {
		return "", "", "", status.Error(codes.InvalidArgument, "weight_kg must be positive")
	}
	return from, to, cargo, nil
}

func scanOrder(row pgx.Row) (*pb.Order, error) {
	var (
		o                pb.Order
		st               string
		created, updated time.Time
		taken, delivered *time.Time
	)
	if err := row.Scan(&o.Id, &o.CreatedBy, &o.OrganizationId, &o.FromAddress, &o.ToAddress,
		&o.CargoDescription, &o.WeightKg, &st, &o.DriverId, &o.VehicleId,
		&created, &updated, &taken, &delivered); err != nil {
		return nil, err
	}
	o.Status = pb.OrderStatus(pb.OrderStatus_value[st])
	o.CreatedAt, o.UpdatedAt = ts(created), ts(updated)
	o.TakenAt, o.DeliveredAt = tsPtr(taken), tsPtr(delivered)
	return &o, nil
}
