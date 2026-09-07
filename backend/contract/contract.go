// Package contract holds every shape and constant that crosses a boundary: the events
// the backend consumes, the messages it publishes to the browser, the thresholds both
// ends depend on, and the service-area geometry both ends draw from (ADR-0001 §1.4,
// §1.8, §1.9).
//
// The TypeScript in the web client is generated from the three message types in wire.go and
// everything reachable from them. Inbound events are backend-only: the client never sees one.
package contract

// Point is a WGS84 coordinate in GeoJSON order — longitude first, then latitude. One
// order is used for every coordinate in the system, because both ends of the pipeline
// already speak it: the checked-in geometry is GeoJSON, and MapLibre consumes GeoJSON.
// There is therefore nowhere for latitude and longitude to be transposed.
type Point [2]float64

func (p Point) Lng() float64 { return p[0] }
func (p Point) Lat() float64 { return p[1] }

// VehicleStatus is what a vehicle is doing right now. These three are the whole of it:
// there is no fourth status, and no sub-distinction within EN_ROUTE between travelling
// towards a customer and away from one (PRODUCT-SPEC §2.1, §4.2).
type VehicleStatus string

const (
	// StatusFree is parked and available. Nobody is driving it.
	StatusFree VehicleStatus = "FREE"
	// StatusEnRoute is a remote driver driving it along a planned route, towards a
	// customer or away again.
	StatusEnRoute VehicleStatus = "EN_ROUTE"
	// StatusWithCustomer is the customer driving it themselves. This is why there is no
	// route to display: the path is the customer's choice, not a plan the system holds.
	StatusWithCustomer VehicleStatus = "WITH_CUSTOMER"
)

// Route is the path a remotely-driven vehicle intends to take. It is shared vocabulary rather
// than two similar shapes because it genuinely is one thing travelling through: a route
// assignment event carries exactly what the client draws, so nothing transforms it on the way
// (ADR-0003 §3.10).
//
// There is no progress field. Progress is a spatial judgement the operator makes from the
// vehicle's position along the drawn line, so a number here would be a second and disagreeing
// source of truth for it.
type Route struct {
	RouteID  string  `json:"routeId"`
	Geometry []Point `json:"geometry"`
	// Destination is what a bare polyline cannot supply: which end of the path is the goal
	// (PRODUCT-SPEC §7.3).
	Destination Point `json:"destination"`
}
