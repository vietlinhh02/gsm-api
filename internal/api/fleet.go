package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const pickupRadiusMeters = 10000

type simulatedDriver struct {
	ID, Name, Phone, Mode, Model, Plate string
	Latitude, Longitude, Distance       float64
}

type fleetQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func nearbyDrivers(
	ctx context.Context, db fleetQuery, environment string, pickup Point,
) ([]simulatedDriver, error) {
	query := `SELECT d.id,d.name,d.phone_number,d.travel_mode,d.latitude,d.longitude,
		d.model,d.license_plate FROM simulated_drivers d
		WHERE d.enabled AND NOT EXISTS (
			SELECT 1 FROM orders o WHERE o.driver_id=d.id AND o.environment=$1
			AND o.status IN ('ASSIGNED','IN PROCESS')) ORDER BY d.id`
	rows, err := db.QueryContext(ctx, query, environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	drivers := []simulatedDriver{}
	for rows.Next() {
		var driver simulatedDriver
		if err := rows.Scan(&driver.ID, &driver.Name, &driver.Phone, &driver.Mode,
			&driver.Latitude, &driver.Longitude, &driver.Model, &driver.Plate); err != nil {
			return nil, err
		}
		driver.Distance = haversine(pickup.Latitude, pickup.Longitude,
			driver.Latitude, driver.Longitude)
		if driver.Distance <= pickupRadiusMeters {
			drivers = append(drivers, driver)
		}
	}
	sort.SliceStable(drivers, func(i, j int) bool { return drivers[i].Distance < drivers[j].Distance })
	return drivers, rows.Err()
}

func orderPoints(payload map[string]any) ([]Point, error) {
	encoded, err := json.Marshal(payload["path"])
	if err != nil {
		return nil, err
	}
	var points []Point
	if err := json.Unmarshal(encoded, &points); err != nil {
		return nil, fmt.Errorf("decode order path: %w", err)
	}
	return points, nil
}

func orderPickup(payload map[string]any) (Point, error) {
	points, err := orderPoints(payload)
	if err != nil {
		return Point{}, err
	}
	for _, point := range points {
		if point.Type == "PICK UP" {
			return point, nil
		}
	}
	return Point{}, fmt.Errorf("order has no pickup point")
}

func applyPointStatus(point *Point, status string) {
	point.Status = status
	if status == "ARRIVED" {
		point.ArriveTime = fmt.Sprint(time.Now().Unix())
	}
	if status == "COMPLETED" {
		point.ProofOfWork = &GPS{Latitude: point.Latitude, Longitude: point.Longitude}
	}
}

func assignDriver(payload map[string]any, driver simulatedDriver) {
	payload["driver_lock_time"] = fmt.Sprint(time.Now().Unix())
	payload["driver"] = map[string]any{
		"id": driver.ID, "name": driver.Name, "phone_number": driver.Phone,
		"rating": 5.0, "is_device_active": true,
	}
	vehicleType := "car"
	if driver.Mode == "Motorcycle" {
		vehicleType = "motorcycle"
	}
	payload["vehicle"] = map[string]any{
		"model": driver.Model, "type": vehicleType, "license_plate": driver.Plate,
		"vehicle_color_name": "XANH", "has_camera": false,
	}
	payload["eta_pickup"] = pickupMinutes(driver.Distance)
}

func pickupMinutes(distance float64) int {
	return max(1, int(math.Ceil(distance/25000*60)))
}

func (a *API) reserveDriver(rctx context.Context, tx *sql.Tx,
	payload map[string]any, environment string, excluded []string,
) (*simulatedDriver, error) {
	pickup, err := orderPickup(payload)
	if err != nil {
		return nil, err
	}
	drivers, err := nearbyDrivers(rctx, tx, environment, pickup)
	if err != nil {
		return nil, err
	}
	for _, driver := range drivers {
		if driver.Mode != payload["travel_mode"] || slices.Contains(excluded, driver.ID) {
			continue
		}
		claimed, err := claimDriver(rctx, tx, driver.ID, environment)
		if err != nil {
			return nil, err
		}
		if claimed {
			return &driver, nil
		}
	}
	return nil, nil
}

func claimDriver(ctx context.Context, tx *sql.Tx, id, environment string) (bool, error) {
	var lockedID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM simulated_drivers
		WHERE id=$1 AND enabled FOR UPDATE SKIP LOCKED`, id).Scan(&lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var occupied bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orders
		WHERE driver_id=$1 AND environment=$2 AND status IN ('ASSIGNED','IN PROCESS'))`,
		id, environment).Scan(&occupied)
	return !occupied, err
}
