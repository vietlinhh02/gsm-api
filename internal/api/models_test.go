package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidatePoints(t *testing.T) {
	tests := []struct {
		name        string
		points      []Point
		requireDrop bool
		wantError   bool
	}{
		{
			name: "pickup and dropoff",
			points: []Point{
				{Type: "PICK UP", Latitude: 10, Longitude: 106, Name: "A", PhoneNumber: "+84123"},
				{Type: "DROP OFF", Latitude: 11, Longitude: 107, Name: "A", PhoneNumber: "+84123"},
			},
			requireDrop: true,
		},
		{
			name: "hour package pickup only",
			points: []Point{
				{Type: "PICK UP", Latitude: 10, Longitude: 106, Name: "A", PhoneNumber: "+84123"},
			},
		},
		{
			name: "missing pickup",
			points: []Point{
				{Type: "DROP OFF", Latitude: 10, Longitude: 106, Name: "A", PhoneNumber: "+84123"},
			},
			wantError: true,
		},
		{
			name: "bad latitude",
			points: []Point{
				{Type: "PICK UP", Latitude: 91, Longitude: 106, Name: "A", PhoneNumber: "+84123"},
			},
			wantError: true,
		},
		{
			name: "wrong point spelling",
			points: []Point{
				{Type: "PICK_UP", Latitude: 10, Longitude: 106, Name: "A", PhoneNumber: "+84123"},
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePoints(test.points, test.requireDrop, false)
			if (err != nil) != test.wantError {
				t.Fatalf("validatePoints() error = %v, want error %v", err, test.wantError)
			}
		})
	}
}

func TestValidateGreenConnectRequiresExactPickupAndBilling(t *testing.T) {
	input := CreateOrderInput{
		PaymentMethod: "partner_pay", PartnerPaymentMethod: "B2B",
		Points: []Point{
			{Type: "PICK UP", AddressNote: "Đón khách A - +84123"},
			{Type: "DROP OFF", AddressNote: "Trả khách A - +84123"},
			{Type: "DROP OFF", AddressNote: "Trả khách B - +84456"},
		},
	}
	if err := validateGreenConnect(input); err != nil {
		t.Fatalf("valid Green Connect request rejected: %v", err)
	}
	input.PaymentMethod = "cash"
	if err := validateGreenConnect(input); err == nil {
		t.Fatal("cash Green Connect request was accepted")
	}
	input.PaymentMethod = "partner_pay"
	input.Points = append(input.Points, Point{Type: "PICK UP", AddressNote: "Đón khách C - +84789"})
	if err := validateGreenConnect(input); err == nil {
		t.Fatal("multiple Green Connect pickups were accepted")
	}
}

func TestOrderTimeBoundaries(t *testing.T) {
	if _, err := normalizeOrderTime(-1); err == nil {
		t.Fatal("negative order time was accepted")
	}
	if _, err := normalizeOrderTime(time.Now().Add(15 * 24 * time.Hour).Unix()); err == nil {
		t.Fatal("order time beyond 14 days was accepted")
	}
	if actual, err := normalizeOrderTime(0); err != nil || actual <= 0 {
		t.Fatalf("zero order time should resolve to now: actual=%d err=%v", actual, err)
	}
}

func TestAddonValidationSeparatesPreviewFromBooking(t *testing.T) {
	previewCost, err := validateAddons([]Addon{{ID: 10, Apply: false}}, true)
	if err != nil || previewCost != 0 {
		t.Fatalf("preview addon should not add to fare: cost=%d err=%v", previewCost, err)
	}
	if _, err := validateAddons([]Addon{{ID: 10, Apply: false}}, false); err == nil {
		t.Fatal("unapplied addon was accepted when creating order")
	}
	if _, err := validateAddons([]Addon{{ID: 99, Apply: true}}, true); err == nil {
		t.Fatal("unknown addon ID was accepted")
	}
}

func TestWebhookSignatureUsesHMACSHA256Hex(t *testing.T) {
	payload := []byte(`{"event_type":"order_status_changed"}`)
	mac := hmac.New(sha256.New, []byte("test-signing-key"))
	_, _ = mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	if actual := webhookSignature(payload, "test-signing-key"); actual != want {
		t.Fatalf("signature = %q, want %q", actual, want)
	}
}

func TestWebhookDeliveryWithoutURLIsNoop(t *testing.T) {
	service := &API{}
	if err := service.deliverOneWebhook(context.Background()); err != nil {
		t.Fatalf("webhook delivery without a URL should be skipped: %v", err)
	}
}

func TestMapPlaceMapsOSMAddressAndRejectsMalformedCoordinates(t *testing.T) {
	place := nominatimPlace{
		OSMType: "way", OSMID: 1234, Lat: "10.7", Lon: "106.6", DisplayName: "Road, Ho Chi Minh City",
		Importance: 0.7, Category: "highway", Type: "residential",
		Address: map[string]string{
			"road":          "Road",
			"suburb":        "Ward",
			"city_district": "District",
			"city":          "Ho Chi Minh City",
		},
	}
	result, err := mapPlace(place, "vi")
	if err != nil {
		t.Fatalf("mapPlace returned error: %v", err)
	}
	if result.PlaceID != "W1234" || result.Street != "Road" || result.Neighborhood != "Ward" ||
		result.Municipality != "District" ||
		result.SubRegion != "Ho Chi Minh City" {
		t.Fatalf("unexpected mapped result: %+v", result)
	}
	place.Lat = "invalid"
	if _, err := mapPlace(place, "vi"); err == nil {
		t.Fatal("malformed latitude was accepted")
	}
}

func TestOSMReverseGeocodeParsesSinglePlace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reverse" || r.URL.Query().Get("lat") != "10.70000000" {
			t.Errorf("unexpected reverse request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(
			w,
			`{"osm_type":"node","osm_id":321,"lat":"10.7","lon":"106.6",`+
				`"display_name":"Test place","category":"place",`+
				`"type":"house","address":{"road":"Test Road"}}`,
		)
	}))
	defer server.Close()
	client := &http.Client{Timeout: time.Second}
	provider := NewOSM(server.URL, server.URL, client)
	latitude, longitude := 10.7, 106.6
	results, err := provider.geocode(context.Background(), geocodeInput{
		Lat: &latitude, Lng: &longitude, Max: 5, Language: "vi",
	})
	if err != nil {
		t.Fatalf("reverse geocode returned error: %v", err)
	}
	if len(results) != 1 || results[0].PlaceID != "N321" || results[0].Street != "Test Road" {
		t.Fatalf("unexpected reverse geocode results: %#v", results)
	}
}
