package api

import (
	"math"
	"testing"
)

func TestHanoiFare(t *testing.T) {
	cases := []struct {
		name            string
		mode            string
		meters, seconds float64
		want            int64
	}{
		{"car minimum", "Car", 0, 0, 30500},
		{"car opening boundary", "Car", 2000, 600, 30500},
		{"car fractional km", "Car", 2500, 600, 38250},
		{"car first tier boundary", "Car", 12000, 600, 185500},
		{"car second tier", "Car", 12500, 600, 192750},
		{"car second tier boundary", "Car", 25000, 600, 374000},
		{"car long distance", "Car", 26000, 600, 386500},
		{"bike minimum", "Motorcycle", 0, 0, 13200},
		{"bike time within opening distance", "Motorcycle", 2000, 600, 15950},
		{"bike distance and time", "Motorcycle", 4800, 600, 27710},
		{"bike fractional minute", "Motorcycle", 2000, 30, 13338},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fare, err := calculateHanoiFare(tc.mode, tc.meters, tc.seconds)
			if err != nil {
				t.Fatal(err)
			}
			if fare.Total != tc.want {
				t.Fatalf("total = %d, want %d", fare.Total, tc.want)
			}
			var total int64
			for _, line := range fare.Items {
				total += line.Amount
			}
			if total != fare.Total {
				t.Fatalf("breakdown sums to %d, total %d", total, fare.Total)
			}
			if !fare.Simulated || fare.City != "Hà Nội" || fare.SourceDate != "2025-03" {
				t.Fatalf("missing reference provenance: %+v", fare)
			}
		})
	}
}

func TestHanoiFareRejectsInvalidRoutes(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1), 1e15} {
		if _, err := calculateHanoiFare("Car", value, 0); err == nil {
			t.Errorf("accepted invalid distance %v", value)
		}
		if _, err := calculateHanoiFare("Motorcycle", 0, value); err == nil {
			t.Errorf("accepted invalid duration %v", value)
		}
	}
	if _, err := calculateHanoiFare("Helicopter", 1000, 60); err == nil {
		t.Fatal("accepted unsupported travel mode")
	}
}
