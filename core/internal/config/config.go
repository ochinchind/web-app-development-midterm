package config

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

type Config struct {
	GRPCPort    string
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
}

func Load() (Config, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	ttl, err := time.ParseDuration(getenv("JWT_TTL", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid JWT_TTL: %w", err)
	}

	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(getenv("POSTGRES_USER", "cargo"), os.Getenv("POSTGRES_PASSWORD")),
		Host:   getenv("DB_HOST", "localhost") + ":" + getenv("DB_PORT", "5432"),
		Path:   getenv("POSTGRES_DB", "cargo"),
	}
	dsn.RawQuery = "sslmode=disable"

	return Config{
		GRPCPort:    getenv("GRPC_PORT", "9090"),
		DatabaseURL: dsn.String(),
		JWTSecret:   secret,
		JWTTTL:      ttl,
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
