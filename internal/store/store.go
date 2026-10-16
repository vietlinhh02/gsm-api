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
	if err := s.seedServices(ctx); err != nil {
		return err
	}
	return s.seedDrivers(ctx)
}

func (s *Store) seedDrivers(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO simulated_drivers
		(id,name,phone_number,travel_mode,latitude,longitude,model,license_plate) VALUES
		('hn-car-1','Tài xế ô tô Hà Nội 1','+84900000001','Car',21.029,105.851,
		 'VF5','29A-000.01 (TEST)'),
		('hn-car-2','Tài xế ô tô Hà Nội 2','+84900000002','Car',21.033,105.818,
		 'VF6','29A-000.02 (TEST)'),
		('hn-bike-1','Tài xế xe máy Hà Nội 1','+84900000003','Motorcycle',21.028,105.85,
		 'VinFast Feliz S','29M1-000.01 (TEST)'),
		('hn-bike-2','Tài xế xe máy Hà Nội 2','+84900000004','Motorcycle',21.032,105.819,
		 'VinFast Evo200','29M1-000.02 (TEST)'),
		('hcm-car-1','Tài xế ô tô HCM 1','+84900000005','Car',10.777,106.701,
		 'VF5','50A-000.01 (TEST)'),
		('hcm-car-2','Tài xế ô tô HCM 2','+84900000006','Car',10.78,106.703,
		 'VF6','50A-000.02 (TEST)'),
		('hcm-bike-1','Tài xế xe máy HCM 1','+84900000007','Motorcycle',10.777,106.701,
		 'VinFast Feliz S','50M1-000.01 (TEST)'),
		('hcm-bike-2','Tài xế xe máy HCM 2','+84900000008','Motorcycle',10.78,106.703,
		 'VinFast Evo200','50M1-000.02 (TEST)')
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed simulated drivers: %w", err)
	}
	return nil
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
