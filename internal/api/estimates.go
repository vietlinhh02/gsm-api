package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type quote struct {
	FeeID         string
	ClientID      string
	ServiceID     int64
	ServiceType   string
	OrderTime     int64
	PaymentMethod string
	Environment   string
	Addons        []Addon
	Points        []Point
	Amount        int64
	Distance      float64
	Duration      float64
	ExpiresAt     int64
	Response      map[string]any
}

func (a *API) estimateTrip(w http.ResponseWriter, r *http.Request) {
	var input EstimateInput
	if !a.readJSON(w, r, &input) {
		return
	}
	if len(input.ServiceIDs) == 0 {
		writeError(w, http.StatusBadRequest, 400, "service_ids must not be empty")
		return
	}
	if err := validatePoints(input.Points, true, false); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	if !validPayment(input.PaymentMethod) {
		writeError(w, http.StatusBadRequest, 400, "payment_method must be cash or partner_pay")
		return
	}
	orderTime, err := normalizeOrderTime(input.OrderTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	route, err := a.OSM.route(r.Context(), input.Points)
	if err != nil {
		writeError(w, http.StatusBadGateway, 502, "could not calculate route using OSM provider")
		return
	}
	estimates := make([]map[string]any, 0, len(input.ServiceIDs))
	for _, selected := range input.ServiceIDs {
		if selected.Addons == nil {
			selected.Addons = []Addon{}
		}
		if _, err := validateAddons(selected.Addons, true); err != nil {
			writeError(w, http.StatusBadRequest, 400, err.Error())
			return
		}
		item, err := a.buildTripEstimate(r, tripQuoteInput{
			ServiceID: selected.ID, OrderTime: orderTime, Payment: input.PaymentMethod,
			Points: input.Points, Addons: selected.Addons, Route: route,
		})
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusBadRequest, 400, "unknown service_id")
				return
			}
			a.dbError(w, "create trip estimate", err)
			return
		}
		estimates = append(estimates, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"list_optimize": nil, "payment_method": input.PaymentMethod,
		"order_time": fmt.Sprint(orderTime), "promo_code": "", "points": echoPoints(input.Points),
		"service_estimates": estimates,
	}})
}

type tripQuoteInput struct {
	ServiceID int64
	OrderTime int64
	Payment   string
	Points    []Point
	Addons    []Addon
	Route     routeData
}

func (a *API) buildTripEstimate(r *http.Request, input tripQuoteInput) (map[string]any, error) {
	var service Service
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT id,display_name,service_type,travel_mode,capacity
		FROM services WHERE id=$1
	`, input.ServiceID).Scan(
		&service.ID,
		&service.DisplayName,
		&service.ServiceType,
		&service.TravelMode,
		&service.Capacity,
	)
	if err != nil {
		return nil, err
	}
	if service.ServiceType != "RIDE-TRIP" {
		return map[string]any{
			"fee_unavailable": true,
			"err_msg":         "service type requires hourly estimate",
			"service":         estimateService(service),
			"addons":          []any{},
		}, nil
	}
	if len(input.Addons) > 0 && service.TravelMode != "Car" {
		return nil, fmt.Errorf("addons are unavailable for this service")
	}
	addonCost, err := validateAddons(input.Addons, true)
	if err != nil {
		return nil, err
	}
	fare, err := calculateHanoiFare(service.TravelMode, input.Route.Distance, input.Route.Duration)
	if err != nil {
		return nil, err
	}
	amount := fare.Total + addonCost
	feeID, err := newUUID()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(10 * time.Minute).Unix()
	quoteResponse := tripEstimateInfo(feeID, expires, input, fare)
	pointsJSON, _ := json.Marshal(input.Points)
	addonsJSON, _ := json.Marshal(input.Addons)
	responseJSON, _ := json.Marshal(quoteResponse)
	_, err = a.Store.DB.ExecContext(
		r.Context(),
		`INSERT INTO estimates(
			fee_id,environment,client_id,service_id,service_type,order_time,
			payment_method,addons,points,amount,route_distance,route_duration,
			response,expires_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		feeID,
		requestEnvironment(r.Context()),
		clientID(r.Context()),
		input.ServiceID,
		service.ServiceType,
		input.OrderTime,
		input.Payment,
		addonsJSON,
		pointsJSON,
		amount,
		input.Route.Distance,
		input.Route.Duration,
		responseJSON,
		expires,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"fee_unavailable": false,
		"err_msg":         "",
		"service":         estimateService(service),
		"addons":          addonEstimates(input.Addons),
		"estimate_info":   quoteResponse,
	}, nil
}

func (a *API) estimateHour(w http.ResponseWriter, r *http.Request) {
	var input HourEstimateInput
	if !a.readJSON(w, r, &input) {
		return
	}
	if len(input.Data) != 1 {
		writeError(
			w,
			http.StatusBadRequest,
			400,
			"data must contain exactly one hourly estimate request",
		)
		return
	}
	request := input.Data[0]
	if request.ID != "" || request.ServiceID <= 0 || request.Hour <= 0 {
		writeError(
			w,
			http.StatusBadRequest,
			400,
			"id must be empty and service_id and hour must be positive",
		)
		return
	}
	if request.Discount < 0 || request.ExpiredAt < 0 || request.Distance < 0 ||
		!validPayment(request.PaymentMethod) {
		writeError(
			w,
			http.StatusBadRequest,
			400,
			"discount, expired_at, distance, or payment_method is invalid",
		)
		return
	}
	if err := validatePoints(request.Points, false, false); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	orderTime, err := normalizeOrderTime(request.OrderTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	var service Service
	err = a.Store.DB.QueryRowContext(r.Context(), `
		SELECT id,display_name,service_type,travel_mode,capacity
		FROM services WHERE id=$1
	`, request.ServiceID).Scan(
		&service.ID,
		&service.DisplayName,
		&service.ServiceType,
		&service.TravelMode,
		&service.Capacity,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			a.dbError(w, "load hourly service", err)
			return
		}
		writeError(w, http.StatusBadRequest, 400, "service_id must identify an hourly service")
		return
	}
	if service.ServiceType != "RIDE-HOUR" {
		writeError(w, http.StatusBadRequest, 400, "service_id must identify an hourly service")
		return
	}
	feeID, err := newUUID()
	if err != nil {
		a.dbError(w, "generate hourly fee id", err)
		return
	}
	amount := request.Hour*175000 - request.Discount
	if amount < 0 {
		amount = 0
	}
	expires := time.Now().Add(10 * time.Minute).Unix()
	pointsJSON, _ := json.Marshal(request.Points)
	responseItem := hourEstimateInfo(feeID, expires, request.Hour, amount, request.Discount)
	responseJSON, _ := json.Marshal(responseItem)
	quoteOrderTime := orderTime
	if request.OrderTime == 0 {
		quoteOrderTime = 0
	}
	_, err = a.Store.DB.ExecContext(
		r.Context(),
		`INSERT INTO estimates(
			fee_id,environment,client_id,service_id,service_type,order_time,
			payment_method,points,amount,response,expires_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		feeID,
		requestEnvironment(r.Context()),
		clientID(r.Context()),
		service.ID,
		service.ServiceType,
		quoteOrderTime,
		request.PaymentMethod,
		pointsJSON,
		amount,
		responseJSON,
		expires,
	)
	if err != nil {
		a.dbError(w, "store hourly estimate", err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"data": map[string]any{"service_estimates": []any{map[string]any{
			"fee_unavailable":   false,
			"service":           estimateService(service),
			"sap_contract_type": "",
			"estimate_infos":    []any{responseItem},
		}}}},
	)
}

func (a *API) loadQuote(ctx *http.Request, feeID string) (quote, error) {
	var result quote
	var addonJSON, pointJSON []byte
	err := a.Store.DB.QueryRowContext(ctx.Context(), `
		SELECT fee_id,client_id,service_id,service_type,order_time,payment_method,
		environment,addons,points,amount,route_distance,route_duration,expires_at,response
		FROM estimates WHERE fee_id=$1
	`, feeID).Scan(
		&result.FeeID, &result.ClientID, &result.ServiceID, &result.ServiceType,
		&result.OrderTime, &result.PaymentMethod, &result.Environment,
		&addonJSON, &pointJSON, &result.Amount, &result.Distance,
		&result.Duration, &result.ExpiresAt, jsonScanner{target: &result.Response},
	)
	if err != nil {
		return quote{}, err
	}
	if err := json.Unmarshal(addonJSON, &result.Addons); err != nil {
		return quote{}, fmt.Errorf("decode stored estimate addons: %w", err)
	}
	if err := json.Unmarshal(pointJSON, &result.Points); err != nil {
		return quote{}, fmt.Errorf("decode stored estimate points: %w", err)
	}
	return result, nil
}

func normalizeOrderTime(value int64) (int64, error) {
	now := time.Now().Unix()
	if value < 0 {
		return 0, fmt.Errorf("order_time must be zero or a future epoch timestamp")
	}
	if value == 0 {
		return now, nil
	}
	if value > now+14*24*60*60 {
		return 0, fmt.Errorf("order_time cannot be more than 14 days in the future")
	}
	return value, nil
}

func validPayment(value string) bool {
	return value == "cash" || value == "partner_pay"
}
