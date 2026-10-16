package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const photonFixture = `{"features":[{"geometry":{"coordinates":[105.8359533,21.0287903]},` +
	`"properties":{"osm_type":"R","osm_id":123,"name":"Văn Miếu",` +
	`"city":"Hà Nội","country":"Việt Nam","osm_key":"amenity","osm_value":"place_of_worship"}}]}`

func TestPhotonFallbackSearchAndReverse(t *testing.T) {
	nominatim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer nominatim.Close()
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "search", true: "reverse"}[reverse], func(t *testing.T) {
			photon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if reverse && (r.URL.Path != "/reverse" || r.URL.Query().Get("lat") == "") {
					t.Errorf("unexpected reverse request: %s", r.URL.String())
				}
				if !reverse && (r.URL.Path != "/api/" || r.URL.Query().Get("countrycode") != "VN") {
					t.Errorf("search must restrict country: %s", r.URL.String())
				}
				io.WriteString(w, photonFixture)
			}))
			defer photon.Close()
			osm := NewOSM(nominatim.URL, nominatim.URL, nominatim.Client())
			osm.Photon = photon.URL
			input := geocodeInput{Text: "Văn Miếu", Max: 5, Language: "vi"}
			if reverse {
				lat, lng := 21.03, 105.84
				input.Lat, input.Lng = &lat, &lng
			}
			results, err := osm.geocode(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].PlaceID != "R123" ||
				results[0].Latitude != 21.0287903 || !strings.Contains(results[0].Label, "Văn Miếu") {
				t.Fatalf("unexpected Photon result: %+v", results)
			}
			if reverse && results[0].Distance <= 0 {
				t.Fatal("missing reverse distance")
			}
		})
	}
}

func TestPhotonFallbackErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"provider failure", 503, `{}`},
		{"invalid json", 200, `{`},
		{"missing coordinates", 200, `{"features":[{"geometry":{"coordinates":[]}}]}`},
		{"invalid coordinates", 200, `{"features":[{"geometry":{"coordinates":[105,91]}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer provider.Close()
			osm := NewOSM(provider.URL, provider.URL, provider.Client())
			osm.Photon = provider.URL
			input := geocodeInput{Text: "test", Max: 5}
			if _, err := osm.photonGeocode(context.Background(), input); err == nil {
				t.Fatal("accepted invalid Photon response")
			}
		})
	}
}

func TestPhotonFallbackRespectsPrivateProviderAndCancellation(t *testing.T) {
	private := NewOSM("http://private.example", "", http.DefaultClient)
	if private.Photon != "" {
		t.Fatal("private providers must not forward queries to public Photon")
	}
	public := NewOSM("https://nominatim.openstreetmap.org/", "", http.DefaultClient)
	if public.Photon != "https://photon.komoot.io" {
		t.Fatal("public provider missing fallback")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := public.geocode(ctx, geocodeInput{Text: "test", Max: 5})
	if err == nil || strings.Contains(err.Error(), "Photon fallback") {
		t.Fatalf("cancelled request must not fall through to Photon: %v", err)
	}
}
