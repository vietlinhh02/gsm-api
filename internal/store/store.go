package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schema embed.FS

type Store struct {
	DB *sql.DB
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &Store{DB: db}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	contents, err := schema.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, string(contents)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return s.seedServices(ctx)
}

func (s *Store) seedServices(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO services (id, display_name, service_type, travel_mode, capacity)
		VALUES
			(2179, 'Green Car', 'RIDE-TRIP', 'Car', 4),
			(2180, 'Green Motorcycle', 'RIDE-TRIP', 'Motorcycle', 1),
			(1903, 'Green Car Hourly', 'RIDE-HOUR', 'Car', 4)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed services: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if err := s.DB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
