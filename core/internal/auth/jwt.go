package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Manager struct {
	secret []byte
	ttl    time.Duration
}

func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl}
}

func (m *Manager) TTL() time.Duration { return m.ttl }

func (m *Manager) Issue(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Manager) Parse(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("token has no subject")
	}
	return claims.Subject, nil
}

type userKey struct{}

func UserID(ctx context.Context) (string, error) {
	id, ok := ctx.Value(userKey{}).(string)
	if !ok || id == "" {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	return id, nil
}

func Interceptor(m *Manager, public map[string]bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return next(ctx, req)
		}

		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}
		token, found := strings.CutPrefix(values[0], "Bearer ")
		if !found {
			return nil, status.Error(codes.Unauthenticated, "authorization header must be 'Bearer <token>'")
		}

		userID, err := m.Parse(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return next(context.WithValue(ctx, userKey{}, userID), req)
	}
}
