package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Service struct {
	ID                    int64    `json:"id"`
	Ordering              int      `json:"ordering"`
	AvailablePickupAreas  []Area   `json:"available_pickup_areas"`
	AvailableDropoffAreas []Area   `json:"available_dropoff_areas"`
	AvailableAreas        []Area   `json:"available_areas"`
	DisplayName           string   `json:"display_name"`
	ServiceType           string   `json:"service_type"`
	Description           string   `json:"description"`
	Capacity              int      `json:"capacity"`
	MaxExtraStop          int      `json:"max_extra_stop"`
	RequireDestination    bool     `json:"require_destination"`
	AutoCancelInterval    int      `json:"auto_cancel_interval"`
	MaxDistance           int      `json:"max_distance"`
	MinDistance           int      `json:"min_distance"`
	Enable                bool     `json:"enable"`
	MaxPassenger          int      `json:"max_passenger"`
	TravelMode            string   `json:"travel_mode"`
	VehicleModels         []string `json:"vehicle_models"`
	ETAPickup             int      `json:"eta_pickup"`
	AvailableVehicles     int      `json:"available_vehicles"`
	DuringService         bool     `json:"during_service"`
}

type Area struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	CountryID int    `json:"country_id"`
}

type ServiceAddon struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	DisplayName       string `json:"display_name"`
	Description       string `json:"description"`
	Type              string `json:"type"`
	Enable            bool   `json:"enable"`
	Ordering          int    `json:"ordering"`
	Group             int    `json:"group"`
	GroupName         string `json:"group_name"`
	DisplayGroupCode  string `json:"display_group_code"`
	DriverDisplayName string `json:"driver_display_name"`
	DriverDescription string `json:"driver_description"`
	AutoApply         bool   `json:"auto_apply"`
}

type serviceRequest struct {
	Pickups      []Point  `json:"pickups"`
	Dropoffs     []Point  `json:"dropoffs"`
	ServiceTypes []string `json:"service_types"`
	OrderTime    int64    `json:"order_time"`
}

func (request serviceRequest) validate() error {
	if len(request.Pickups) == 0 {
		return fmt.Errorf("pickups must contain at least one point")
	}
	if request.OrderTime < 0 {
		return fmt.Errorf("order_time must be zero or a positive epoch timestamp")
	}
	for _, serviceType := range request.ServiceTypes {
		if strings.TrimSpace(serviceType) == "" {
			return fmt.Errorf("service_types values must not be empty")
		}
	}
	for _, point := range append(append([]Point{}, request.Pickups...), request.Dropoffs...) {
		if !validCoordinate(point.Latitude, -90, 90) ||
			!validCoordinate(point.Longitude, -180, 180) {
			return fmt.Errorf("pickup/dropoff coordinates must be within latitude/longitude bounds")
		}
	}
	return nil
}

func (a *API) listServices(w http.ResponseWriter, r *http.Request) {
	var request serviceRequest
	if !a.readJSON(w, r, &request) {
		return
	}
	if err := request.validate(); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	drivers, err := nearbyDrivers(r.Context(), a.Store.DB,
		requestEnvironment(r.Context()), request.Pickups[0])
	if err != nil {
		a.dbError(w, "check nearby vehicles", err)
		return
	}
	services, err := a.serviceCatalog(r.Context())
	if err != nil {
		a.dbError(w, "list services", err)
		return
	}
	data := make([]map[string]any, 0)
	for _, service := range services {
		if contains(request.ServiceTypes, service.ServiceType) {
			data = append(data, serviceResponse(withAvailability(decorateService(service), drivers)))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "paginate": 0})
}

func (a *API) serviceCatalog(ctx context.Context) ([]Service, error) {
	rows, err := a.Store.DB.QueryContext(ctx,
		`SELECT id,display_name,service_type,travel_mode,capacity FROM services ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	services := []Service{}
	for rows.Next() {
		var service Service
		if err := rows.Scan(&service.ID, &service.DisplayName, &service.ServiceType,
			&service.TravelMode, &service.Capacity); err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	return services, rows.Err()
}

func serviceResponse(service Service) map[string]any {
	addons := []ServiceAddon{}
	if service.TravelMode == "Car" {
		addons = []ServiceAddon{quietAddon()}
	}
	return map[string]any{
		"service": service, "addons": addons,
		"service_hours": []map[string]int{{"day_of_week": 1, "start_time": 0, "end_time": 1440}},
		"service_group": map[string]any{"id": 1, "name": service.TravelMode, "ordering": 1},
		"addon_groups":  []map[string]any{}, "cross_services": []any{},
	}
}

func decorateService(service Service) Service {
	service.Ordering = 1
	service.AvailablePickupAreas = localAreas()
	service.AvailableDropoffAreas = localAreas()
	service.AvailableAreas = localAreas()
	service.MaxExtraStop = 3
	service.RequireDestination = true
	service.AutoCancelInterval = 300
	service.MaxDistance = 999999999
	service.MinDistance = 100
	service.Enable = true
	service.MaxPassenger = service.Capacity
	service.DuringService = true
	service.VehicleModels = []string{}
	if service.TravelMode == "Car" {
		service.VehicleModels = []string{"VF5", "VF6", "VF7", "VF8", "VF9"}
	}
	return service
}

func withAvailability(service Service, drivers []simulatedDriver) Service {
	for _, driver := range drivers {
		if driver.Mode != service.TravelMode {
			continue
		}
		service.AvailableVehicles++
		if service.ETAPickup == 0 {
			service.ETAPickup = pickupMinutes(driver.Distance)
		}
	}
	service.Enable = service.AvailableVehicles > 0
	return service
}

func localAreas() []Area {
	return []Area{{ID: 1, Name: "Hà Nội", CountryID: 1}}
}

func quietAddon() ServiceAddon {
	return ServiceAddon{
		ID:                10,
		Name:              "QUIET_RIDE",
		DisplayName:       "Quiet ride",
		Description:       "The driver keeps conversation to a minimum during the trip",
		Type:              "BOOLEAN",
		Enable:            true,
		Ordering:          1,
		Group:             1,
		GroupName:         "RIDE_PREFERENCE",
		DisplayGroupCode:  "RIDE_PREFERENCE",
		DriverDisplayName: "Quiet ride",
		DriverDescription: "The driver keeps conversation to a minimum during the trip",
	}
}

func contains(values []string, sought string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if strings.EqualFold(value, sought) {
			return true
		}
	}
	return false
}
