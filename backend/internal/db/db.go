package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			getenv("POSTGRES_USER", "sharedgpu"),
			getenv("POSTGRES_PASSWORD", "sharedgpu"),
			getenv("POSTGRES_HOST", "localhost"),
			getenv("POSTGRES_PORT", "5432"),
			getenv("POSTGRES_DB", "sharedgpu"),
		)
	}
	return pgxpool.New(ctx, dsn)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
