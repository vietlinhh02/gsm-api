package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OSM struct {
	Nominatim string
	Photon    string
	OSRM      string
	HTTP      *http.Client
	mu        sync.Mutex
	nextCall  time.Time
}

func NewOSM(nominatim, osrm string, client *http.Client) *OSM {
	provider := &OSM{
		Nominatim: strings.TrimRight(nominatim, "/"),
		OSRM:      strings.TrimRight(osrm, "/"),
		HTTP:      client,
	}
	if provider.Nominatim == "https://nominatim.openstreetmap.org" {
		provider.Photon = "https://photon.komoot.io"
	}
	return provider
}

type GeoResult struct {
	Label         string         `json:"label"`
	PlaceID       string         `json:"place_id"`
	Longitude     float64        `json:"longitude"`
	Latitude      float64        `json:"latitude"`
	Street        string         `json:"street"`
	Neighborhood  string         `json:"neighborhood"`
	Municipality  string         `json:"municipality"`
	SubRegion     string         `json:"sub_region"`
	Distance      float64        `json:"distance"`
	Language      string         `json:"language"`
	IsSnap        bool           `json:"is_snap"`
	IsVerified    bool           `json:"is_verified"`
	Visit         int            `json:"visit"`
	Introduce     bool           `json:"introduce"`
	PlaceTypeOSM  map[string]any `json:"place_type_osm"`
	InReview      *bool          `json:"in_review"`
	IsLocked      *bool          `json:"is_locked"`
	NewLabel      string         `json:"new_label"`
	RedirectPlace bool           `json:"redirect_place"`
	IsIndexed     *bool          `json:"IsIndexed"`
	Timezone      string         `json:"timezone"`
	OrderPosition *int           `json:"order_position"`
	Score         *float64       `json:"Score"`
}

type nominatimPlace struct {
	PlaceID     int64             `json:"place_id"`
	OSMType     string            `json:"osm_type"`
	OSMID       int64             `json:"osm_id"`
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	DisplayName string            `json:"display_name"`
	Category    string            `json:"category"`
	Type        string            `json:"type"`
	Importance  float64           `json:"importance"`
	Address     map[string]string `json:"address"`
}

type geocodeInput struct {
	Text     string
	Lat, Lng *float64
	Max      int
	Language string
}

func (o *OSM) geocode(ctx context.Context, input geocodeInput) ([]GeoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	primaryContext, primaryCancel := context.WithTimeout(ctx, 3*time.Second)
	results, err := o.nominatimGeocode(primaryContext, input)
	primaryCancel()
	if err == nil {
		return results, nil
	}
	if o.Photon == "" || ctx.Err() != nil {
		return nil, err
	}
	results, fallbackErr := o.photonGeocode(ctx, input)
	if fallbackErr != nil {
		return nil, fmt.Errorf("Nominatim failed (%v); Photon fallback: %w", err, fallbackErr)
	}
	return results, nil
}

func nominatimQuery(input geocodeInput) (string, url.Values) {
	query := url.Values{}
	query.Set("format", "jsonv2")
	query.Set("addressdetails", "1")
	query.Set("accept-language", input.Language)
	query.Set("limit", strconv.Itoa(input.Max))
	path := "/search"
	if input.Lat != nil && input.Lng != nil {
		path = "/reverse"
		query.Set("lat", strconv.FormatFloat(*input.Lat, 'f', 8, 64))
		query.Set("lon", strconv.FormatFloat(*input.Lng, 'f', 8, 64))
	} else {
		query.Set("q", input.Text)
		query.Set("countrycodes", "vn")
	}
	return path, query
}

func (o *OSM) nominatimGeocode(ctx context.Context, input geocodeInput) ([]GeoResult, error) {
	path, query := nominatimQuery(input)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		o.Nominatim+path+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create Nominatim request: %w", err)
	}
	request.Header.Set("User-Agent", "GSM-Rides-API-Simulator/1.0 (local development)")
	request.Header.Set("Accept", "application/json")
	if input.Language != "" {
		request.Header.Set("Accept-Language", input.Language)
	}
	if err := o.waitNominatim(ctx); err != nil {
		return nil, err
	}
	response, err := o.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Nominatim geocoding: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return []GeoResult{}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nominatim returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read Nominatim response: %w", err)
	}
	var places []nominatimPlace
	if path == "/reverse" {
		var place nominatimPlace
		if err := json.Unmarshal(body, &place); err != nil {
			return nil, fmt.Errorf("decode Nominatim reverse response: %w", err)
		}
		places = append(places, place)
	} else if err := json.Unmarshal(body, &places); err != nil {
		return nil, fmt.Errorf("decode Nominatim search response: %w", err)
	}
	results := make([]GeoResult, 0, len(places))
	for _, place := range places {
		result, err := mapPlace(place, input.Language)
		if err != nil {
			continue
		}
		if input.Lat != nil && input.Lng != nil {
			result.Distance = haversine(*input.Lat, *input.Lng, result.Latitude, result.Longitude)
		}
		results = append(results, result)
	}
	return results, nil
}

func (o *OSM) waitNominatim(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now()
	if o.nextCall.Before(now) {
		o.nextCall = now
	}
	wait := time.Until(o.nextCall)
	o.nextCall = o.nextCall.Add(time.Second)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for Nominatim rate limit: %w", ctx.Err())
	}
}

func mapPlace(place nominatimPlace, language string) (GeoResult, error) {
	lat, err := strconv.ParseFloat(place.Lat, 64)
	if err != nil {
		return GeoResult{}, fmt.Errorf("parse latitude: %w", err)
	}
	lng, err := strconv.ParseFloat(place.Lon, 64)
	if err != nil {
		return GeoResult{}, fmt.Errorf("parse longitude: %w", err)
	}
	address := place.Address
	osmPrefix := ""
	if len(place.OSMType) > 0 {
		osmPrefix = strings.ToUpper(place.OSMType[:1])
	}
	placeID := fmt.Sprintf("%s%d", osmPrefix, place.OSMID)
	tags := map[string]any{"category": place.Category, "type": place.Type}
	var score *float64
	if place.Importance > 0 {
		score = &place.Importance
	}
	label := place.DisplayName
	return GeoResult{
		Label: label, NewLabel: label, PlaceID: placeID, Latitude: lat, Longitude: lng,
		Street: firstNonEmpty(
			address["road"],
			address["pedestrian"],
			address["footway"],
			address["residential"],
		),
		Neighborhood: firstNonEmpty(
			address["neighbourhood"],
			address["suburb"],
			address["quarter"],
		),
		Municipality: firstNonEmpty(
			address["city_district"],
			address["district"],
			address["county"],
			address["town"],
		),
		SubRegion: firstNonEmpty(address["state"], address["province"], address["city"]),
		Language:  language, PlaceTypeOSM: tags, Timezone: "+7", Score: score,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type routeGeometry struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type routeData struct {
	Distance float64        `json:"distance"`
	Duration float64        `json:"duration"`
	Geometry *routeGeometry `json:"geometry"`
}

func (o *OSM) fetchRoute(ctx context.Context, points []Point) (routeData, error) {
	coordinates := make([]string, 0, len(points))
	for _, point := range points {
		coordinates = append(coordinates, fmt.Sprintf("%f,%f", point.Longitude, point.Latitude))
	}
	endpoint := o.OSRM + "/route/v1/driving/" + strings.Join(coordinates, ";") +
		"?overview=simplified&geometries=geojson"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return routeData{}, fmt.Errorf("create OSRM request: %w", err)
	}
	response, err := o.HTTP.Do(request)
	if err != nil {
		return routeData{}, fmt.Errorf("request OSRM route: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return routeData{}, fmt.Errorf("OSRM returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Code   string      `json:"code"`
		Routes []routeData `json:"routes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return routeData{}, fmt.Errorf("decode OSRM response: %w", err)
	}
	if result.Code != "Ok" || len(result.Routes) == 0 {
		return routeData{}, fmt.Errorf("OSRM did not find a route")
	}
	return result.Routes[0], nil
}

func (o *OSM) route(ctx context.Context, points []Point) (routeData, error) {
	route, err := o.fetchRoute(ctx, points)
	if err != nil {
		return routeData{}, err
	}
	if !validCoordinate(route.Distance, 0, 1e8) || !validCoordinate(route.Duration, 0, 1e8) {
		return routeData{}, fmt.Errorf("OSRM returned invalid route distance or duration")
	}
	if err := validateRouteGeometry(route.Geometry); err != nil {
		return routeData{}, err
	}
	return route, nil
}

func validateRouteGeometry(geometry *routeGeometry) error {
	if geometry == nil {
		return nil
	}
	if geometry.Type != "LineString" || len(geometry.Coordinates) < 2 {
		return fmt.Errorf("OSRM returned invalid route geometry")
	}
	for _, coordinate := range geometry.Coordinates {
		if len(coordinate) != 2 {
			return fmt.Errorf("OSRM returned malformed route coordinates")
		}
		if !validCoordinate(coordinate[0], -180, 180) ||
			!validCoordinate(coordinate[1], -90, 90) {
			return fmt.Errorf("OSRM route coordinates are outside latitude/longitude bounds")
		}
	}
	return nil
}

func geocodeCacheKey(text string, lat, lng *float64, max int, language string) string {
	input := fmt.Sprintf(
		"%s|%v|%v|%d|%s",
		strings.ToLower(strings.TrimSpace(text)),
		lat,
		lng,
		max,
		language,
	)
	digest := sha256.Sum256([]byte(input))
	return hex.EncodeToString(digest[:])
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000
	toRadians := math.Pi / 180
	deltaLat := (lat2 - lat1) * toRadians
	deltaLon := (lon2 - lon1) * toRadians
	a := math.Sin(
		deltaLat/2,
	)*math.Sin(
		deltaLat/2,
	) + math.Cos(
		lat1*toRadians,
	)*math.Cos(
		lat2*toRadians,
	)*math.Sin(
		deltaLon/2,
	)*math.Sin(
		deltaLon/2,
	)
	if a > 1 {
		a = 1
	}
	return earthRadius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
