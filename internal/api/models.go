package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

type Point struct {
	ID           string  `json:"id,omitempty"`
	Type         string  `json:"type"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Name         string  `json:"name"`
	PhoneNumber  string  `json:"phone_number"`
	ShortAddress string  `json:"short_address"`
	Address      string  `json:"address"`
	AddressNote  string  `json:"address_note,omitempty"`
	PlaceID      string  `json:"place_id,omitempty"`
	Status       string  `json:"status,omitempty"`
	ArriveTime   string  `json:"arrive_time,omitempty"`
	ProofOfWork  *GPS    `json:"proof_of_work,omitempty"`
}

type GPS struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Addon struct {
	ID    int64 `json:"id"`
	Apply bool  `json:"apply"`
}

type EstimateInput struct {
	ServiceIDs []struct {
		ID     int64   `json:"id"`
		Addons []Addon `json:"addons"`
	} `json:"service_ids"`
	OrderTime     int64   `json:"order_time"`
	PaymentMethod string  `json:"payment_method"`
	Points        []Point `json:"points"`
}

type HourEstimateInput struct {
	Data []struct {
		ID            string  `json:"id"`
		ServiceID     int64   `json:"service_id"`
		Hour          int64   `json:"hour"`
		OrderTime     int64   `json:"order_time"`
		PromoCode     *string `json:"promo_code"`
		Discount      int64   `json:"discount"`
		PaymentMethod string  `json:"payment_method"`
		ExpiredAt     int64   `json:"expired_at"`
		Distance      int64   `json:"distance"`
		Points        []Point `json:"points"`
	} `json:"data"`
}

type CreateOrderInput struct {
	OrderTime            int64   `json:"order_time"`
	ServiceID            int64   `json:"service_id"`
	ServiceType          string  `json:"service_type"`
	PaymentMethod        string  `json:"payment_method"`
	PartnerPaymentMethod string  `json:"partner_payment_method"`
	FeeID                string  `json:"fee_id"`
	PartnerTripID        string  `json:"partner_trip_id"`
	Addons               []Addon `json:"addons"`
	Points               []Point `json:"points"`
}

func validatePoints(points []Point, requireDropoff bool, requireAddress bool) error {
	if len(points) == 0 {
		return errors.New("at least one route point is required")
	}
	pickups, dropoffs := 0, 0
	for index, point := range points {
		if point.Type == "PICK UP" {
			pickups++
		} else if point.Type == "DROP OFF" {
			dropoffs++
		} else {
			return fmt.Errorf("points[%d].type must be PICK UP or DROP OFF", index)
		}
		if !validCoordinate(point.Latitude, -90, 90) ||
			!validCoordinate(point.Longitude, -180, 180) {
			return fmt.Errorf("points[%d] has coordinates outside valid range", index)
		}
		if strings.TrimSpace(point.Name) == "" || len(point.Name) > 256 {
			return fmt.Errorf(
				"points[%d].name is required and must be at most 256 characters",
				index,
			)
		}
		if !validPhone(point.PhoneNumber) {
			return fmt.Errorf(
				"points[%d].phone_number must include a country code and start with +",
				index,
			)
		}
		missingShortAddress := strings.TrimSpace(point.ShortAddress) == ""
		missingAddress := strings.TrimSpace(point.Address) == ""
		if requireAddress && (missingShortAddress || len(point.ShortAddress) > 512 ||
			missingAddress || len(point.Address) > 512) {
			return fmt.Errorf(
				"points[%d] requires short_address and address, each at most 512 characters",
				index,
			)
		}
		if len(point.AddressNote) > 512 {
			return fmt.Errorf("points[%d].address_note exceeds 512 characters", index)
		}
	}
	if pickups == 0 || (requireDropoff && dropoffs == 0) {
		return errors.New("at least one PICK UP and one DROP OFF are required")
	}
	return nil
}

func validPhone(value string) bool {
	if len(value) < 5 || value[0] != '+' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validateGreenConnect(input CreateOrderInput) error {
	if input.PaymentMethod != "partner_pay" || input.PartnerPaymentMethod != "B2B" {
		return errors.New("Green Connect orders require partner_pay and partner_payment_method B2B")
	}
	pickups, dropoffs := 0, 0
	for _, point := range input.Points {
		if point.Type == "PICK UP" {
			pickups++
			if !strings.HasPrefix(point.AddressNote, "Đón khách ") ||
				!strings.Contains(point.AddressNote, " - ") {
				return errors.New("Green Connect PICK UP address_note must start with 'Đón khách '")
			}
		} else if point.Type == "DROP OFF" {
			dropoffs++
			if !strings.HasPrefix(point.AddressNote, "Trả khách ") ||
				!strings.Contains(point.AddressNote, " - ") {
				return errors.New("Green Connect DROP OFF address_note must start with 'Trả khách '")
			}
		}
	}
	if pickups != 1 || dropoffs < 1 {
		return errors.New("Green Connect requires exactly one PICK UP and at least one DROP OFF")
	}
	return nil
}

func validCoordinate(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func jsonEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
