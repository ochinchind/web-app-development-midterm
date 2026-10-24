package service

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func checkID(id, field string) error {
	if _, err := uuid.Parse(id); err != nil {
		return status.Errorf(codes.InvalidArgument, "%s must be a valid UUID", field)
	}
	return nil
}

func dbErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return status.Error(codes.AlreadyExists, "already exists: "+pgErr.ConstraintName)
		case "23503": // foreign_key_violation
			return status.Error(codes.InvalidArgument, "referenced record does not exist")
		}
	}
	return status.Error(codes.Internal, "database error")
}

func ts(t time.Time) *timestamppb.Timestamp { return timestamppb.New(t) }

func tsPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
