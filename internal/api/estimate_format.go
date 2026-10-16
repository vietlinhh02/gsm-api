package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func estimateService(service Service) map[string]any {
	groupName, groupID := service.TravelMode, 1
	if service.ServiceType == "RIDE-HOUR" {
		groupName, groupID = "Airport", 5
	}
	return map[string]any{
		"id": service.ID, "display_name": service.DisplayName, "description": "",
		"short_description": "", "service_group_name": groupName,
		"service_group_id": groupID, "travel_mode": service.TravelMode,
		"partner_service_type": "", "service_type": service.ServiceType,
	}
}

func tripEstimateInfo(
	feeID string,
	expires int64,
	input tripQuoteInput,
	fare fareDetails,
) map[string]any {
	var addonFee int64
	for _, addon := range input.Addons {
		if addon.Apply {
			addonFee += addonPrice(addon.ID)
		}
	}
	amount := fare.Total + addonFee
	return map[string]any{
		"fee_id": feeID, "increase_price": false, "signature": "",
		"expires_at": expires,
		"pricing":    fare,
		"estimate": map[string]any{
			"display": formatVND(amount), "total_fee_display": formatVND(amount),
			"total_pay_display": formatVND(amount), "total_fee": amount,
			"total_pay": amount, "currency_code": "VND",
			"fee_breakdown": map[string]any{
				"base_fee": fare.Total, "surcharge_fee": 0, "addons_fee": addonFee,
				"discount": 0, "discount_display": "0đ", "surge_multiplier": 1,
				"service_suggestion_label": nil, "origin_amount_apply_voucher": amount,
			},
		},
		"trip": map[string]any{
			"distance_unit":     "km",
			"distance_estimate": input.Route.Distance / 1000,
			"duration_estimate": int64(input.Route.Duration),
			"geometry":          input.Route.Geometry,
			"toll_details":      nil,
		},
	}
}

func hourEstimateInfo(feeID string, expires int64, hours, amount, discount int64) map[string]any {
	return map[string]any{
		"fee_id": feeID, "signature": "", "expires_at": expires,
		"service_package_hour": hours, "service_package_distance": hours * 15,
		"total_fee_display": formatVND(amount), "total_fee": amount,
		"total_pay_display": formatVND(amount), "total_pay": amount,
		"discount": discount, "promo_info": nil, "duration": 0,
		"duration_display": fmt.Sprintf("0/%dPhút", hours*60), "distance": 0,
		"distance_display": fmt.Sprintf("0/%dkm", hours*15),
	}
}

func addonEstimates(addons []Addon) []map[string]any {
	result := make([]map[string]any, 0, len(addons))
	for _, addon := range addons {
		cost := addonPrice(addon.ID)
		result = append(
			result,
			map[string]any{
				"id":           addon.ID,
				"apply":        addon.Apply,
				"cost":         cost,
				"cost_display": formatVND(cost),
			},
		)
	}
	return result
}

func validateAddons(addons []Addon, estimate bool) (int64, error) {
	var total int64
	for _, addon := range addons {
		if addon.ID != 10 && addon.ID != 12 && addon.ID != 13 && addon.ID != 14 {
			return 0, fmt.Errorf("unknown addon id %d", addon.ID)
		}
		if !estimate && !addon.Apply {
			return 0, fmt.Errorf("addons on create-order must have apply=true")
		}
		if addon.Apply {
			total += addonPrice(addon.ID)
		}
	}
	return total, nil
}

func addonPrice(id int64) int64 {
	if id == 10 {
		return 10000
	}
	return 5000
}

func formatVND(amount int64) string {
	text := fmt.Sprintf("%d", amount)
	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + "." + text[index:]
	}
	return text + "đ"
}

func echoPoints(points []Point) []map[string]any {
	result := make([]map[string]any, 0, len(points))
	for _, point := range points {
		result = append(result, map[string]any{
			"type": point.Type, "phone_number": point.PhoneNumber, "name": point.Name,
			"longitude": point.Longitude, "latitude": point.Latitude,
			"address": point.Address, "short_address": point.ShortAddress,
			"place_id": point.PlaceID, "package_weight": 0, "package_size": "",
		})
	}
	return result
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return strings.Join(
		[]string{encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]},
		"-",
	), nil
}
