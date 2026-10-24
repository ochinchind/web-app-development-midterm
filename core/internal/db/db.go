package db

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}

	var pingErr error
	for i := 0; i < 30; i++ {
		if pingErr = pool.Ping(ctx); pingErr == nil {
			return pool, nil
		}
		log.Printf("waiting for database: %v", pingErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("database not reachable: %w", pingErr)
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		sql, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		log.Printf("applied migration %s", name)
	}
	return nil
}
