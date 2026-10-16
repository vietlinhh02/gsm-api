package api

import (
	"fmt"
	"math"
)

const hanoiFareSource = "https://www.xanhsm.com/news/du-lich-sinh-thai-ban-rom-soc-son-ha-noi"

type fareLine struct {
	Label    string  `json:"label"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Rate     int64   `json:"rate"`
	Amount   int64   `json:"amount"`
}

type fareDetails struct {
	City           string     `json:"city"`
	Simulated      bool       `json:"simulated"`
	SourceURL      string     `json:"source_url"`
	SourceDate     string     `json:"source_date"`
	Rounding       string     `json:"rounding"`
	RoutingProfile string     `json:"routing_profile"`
	Items          []fareLine `json:"items"`
	Total          int64      `json:"total"`
}

func calculateHanoiFare(mode string, meters, seconds float64) (fareDetails, error) {
	if !validCoordinate(meters, 0, 1e8) || !validCoordinate(seconds, 0, 1e8) {
		return fareDetails{}, fmt.Errorf("route distance or duration is invalid")
	}
	fare := fareDetails{
		City: "Hà Nội", Simulated: true, SourceURL: hanoiFareSource, SourceDate: "2025-03",
		Rounding:       "Each component rounded to nearest VND; fractional km/minutes prorated",
		RoutingProfile: "OSRM driving (also used as an approximation for motorcycles)",
	}
	km := meters / 1000
	switch mode {
	case "Car":
		fare.Items = []fareLine{makeFareLine("Mở cửa · 2 km đầu", 1, "chuyến", 30500)}
		fare.Items = append(fare.Items,
			makeFareLine("Trên 2 đến 12 km", math.Max(0, math.Min(km, 12)-2), "km", 15500),
			makeFareLine("Trên 12 đến 25 km", math.Max(0, math.Min(km, 25)-12), "km", 14500),
			makeFareLine("Trên 25 km", math.Max(0, km-25), "km", 12500),
		)
	case "Motorcycle":
		fare.Items = []fareLine{
			makeFareLine("Mở cửa · 2 km đầu", 1, "chuyến", 13200),
			makeFareLine("Trên 2 km", math.Max(0, km-2), "km", 4200),
			makeFareLine("Thời gian hành trình", seconds/60, "phút", 275),
		}
	default:
		return fareDetails{}, fmt.Errorf("unsupported travel mode %q", mode)
	}
	for _, line := range fare.Items {
		fare.Total += line.Amount
	}
	return fare, nil
}

func makeFareLine(label string, quantity float64, unit string, rate int64) fareLine {
	return fareLine{
		Label: label, Quantity: quantity, Unit: unit, Rate: rate,
		Amount: int64(math.Round(quantity * float64(rate))),
	}
}
