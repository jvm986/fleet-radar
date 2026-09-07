package derive

import (
	"testing"
	"time"

	"fleetradar/contract"
	"fleetradar/fleet"
)

var published = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// Two zones with a gap between them, so that "in no zone" is reachable without leaving the
// service area — which is the case any aggregation is most likely to drop (PRODUCT-SPEC §7.2).
var zones = []contract.Zone{
	{ID: "west", Name: "West", Minimum: 2, Boundary: rectangle(0, 0, 1, 1)},
	{ID: "east", Name: "East", Minimum: 1, Boundary: rectangle(2, 0, 3, 1)},
}

var (
	inWest       = contract.Point{0.5, 0.5}
	inEast       = contract.Point{2.5, 0.5}
	betweenZones = contract.Point{1.5, 0.5}
	farAway      = contract.Point{50, 50}
)

func rectangle(minLng, minLat, maxLng, maxLat float64) []contract.Point {
	return []contract.Point{
		{minLng, minLat}, {maxLng, minLat}, {maxLng, maxLat}, {minLng, maxLat}, {minLng, minLat},
	}
}

func reporting(id string, status contract.VehicleStatus, position contract.Point, battery float64, silentFor time.Duration) fleet.Vehicle {
	return fleet.Vehicle{
		ID:           id,
		Label:        "LV-" + id,
		Position:     position,
		Heading:      90,
		Status:       status,
		Battery:      battery,
		Reporting:    true,
		LastObserved: published.Add(-silentFor),
	}
}

func TestAttention(t *testing.T) {
	for _, tc := range []struct {
		name      string
		battery   float64
		silentFor time.Duration
		dominant  contract.AttentionReason
		reasons   []contract.AttentionReason
	}{
		{
			name: "a healthy vehicle warrants none", battery: 55, silentFor: 0,
			dominant: contract.AttentionNone, reasons: []contract.AttentionReason{},
		},
		{
			name:    "at the threshold the energy is not low: the legend says below 20%",
			battery: contract.LowBatteryPercent, silentFor: 0,
			dominant: contract.AttentionNone, reasons: []contract.AttentionReason{},
		},
		{
			name: "just below the threshold it is", battery: contract.LowBatteryPercent - 0.1, silentFor: 0,
			dominant: contract.AttentionLowBattery, reasons: []contract.AttentionReason{contract.AttentionLowBattery},
		},
		{
			name: "silence of exactly two reports has not yet missed two", battery: 55, silentFor: contract.StaleAfter,
			dominant: contract.AttentionNone, reasons: []contract.AttentionReason{},
		},
		{
			name: "silence beyond it has", battery: 55, silentFor: contract.StaleAfter + time.Millisecond,
			dominant: contract.AttentionStale, reasons: []contract.AttentionReason{contract.AttentionStale},
		},
		{
			name:    "staleness subsumes low energy, and both are still reported",
			battery: 4, silentFor: contract.StaleAfter * 2,
			dominant: contract.AttentionStale,
			reasons:  []contract.AttentionReason{contract.AttentionStale, contract.AttentionLowBattery},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fleetState := []fleet.Vehicle{reporting("1", contract.StatusFree, inWest, tc.battery, tc.silentFor)}
			vehicle := Snapshot(fleetState, zones, contract.LifecycleReady, published).Vehicles[0]

			if vehicle.Attention != tc.dominant {
				t.Errorf("Attention = %q, want %q", vehicle.Attention, tc.dominant)
			}
			if len(vehicle.AttentionReasons) != len(tc.reasons) {
				t.Fatalf("AttentionReasons = %v, want %v", vehicle.AttentionReasons, tc.reasons)
			}
			for i, reason := range tc.reasons {
				if vehicle.AttentionReasons[i] != reason {
					t.Errorf("AttentionReasons[%d] = %q, want %q", i, vehicle.AttentionReasons[i], reason)
				}
			}
		})
	}
}

// A vehicle known from its registration and never heard from is not stale: we have never had it,
// as opposed to having had it and lost it. Its zero observation timestamp would otherwise read as
// decades of silence (PRODUCT-SPEC F6).
func TestAVehicleAwaitingItsFirstReportIsNotStale(t *testing.T) {
	snapshot := Snapshot([]fleet.Vehicle{{ID: "1", Label: "LV-1"}}, zones, contract.LifecycleReady, published)

	if len(snapshot.Vehicles) != 0 {
		t.Errorf("Vehicles = %+v, want none: it cannot be drawn", snapshot.Vehicles)
	}
	if len(snapshot.AwaitingFirstReport) != 1 || snapshot.AwaitingFirstReport[0].Label != "LV-1" {
		t.Errorf("AwaitingFirstReport = %+v, want the vehicle named", snapshot.AwaitingFirstReport)
	}
	if snapshot.Summary.Stale != 0 {
		t.Errorf("Summary.Stale = %d, want 0", snapshot.Summary.Stale)
	}
	if snapshot.Summary.Total != 1 || snapshot.Summary.AwaitingFirstReport != 1 {
		t.Errorf("Summary = %+v, want it accounted for", snapshot.Summary)
	}
}

// The observation clock is authoritative even when it runs ahead of ours, but "silent for minus
// three seconds" is not something we can say.
func TestSilenceIsNeverNegative(t *testing.T) {
	fleetState := []fleet.Vehicle{reporting("1", contract.StatusFree, inWest, 55, -3*time.Second)}

	if got := Snapshot(fleetState, zones, contract.LifecycleReady, published).Vehicles[0].SilentForMs; got != 0 {
		t.Errorf("SilentForMs = %d, want 0", got)
	}
}

func TestZoneMembership(t *testing.T) {
	for _, tc := range []struct {
		name     string
		position contract.Point
		want     contract.ZoneID
	}{
		{"inside a zone", inWest, "west"},
		{"inside the other", inEast, "east"},
		{"between zones", betweenZones, contract.ZoneNone},
		{"far outside every zone", farAway, contract.ZoneNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fleetState := []fleet.Vehicle{reporting("1", contract.StatusFree, tc.position, 55, 0)}

			if got := Snapshot(fleetState, zones, contract.LifecycleReady, published).Vehicles[0].ZoneID; got != tc.want {
				t.Errorf("ZoneID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCoverage(t *testing.T) {
	// West needs two, East needs one.
	fleetState := []fleet.Vehicle{
		reporting("1", contract.StatusFree, inWest, 55, 0),
		reporting("2", contract.StatusWithCustomer, inWest, 55, 0),
		reporting("3", contract.StatusEnRoute, inEast, 55, 0),
		// Available, but in no zone: it counts towards nothing except the fleet's own total.
		reporting("4", contract.StatusFree, betweenZones, 55, 0),
		// Low energy is a cue to plan a charge, not a declaration that the vehicle cannot do a
		// job, so it still counts as coverage (PRODUCT-SPEC §2.5).
		reporting("5", contract.StatusFree, inWest, 3, 0),
	}
	snapshot := Snapshot(fleetState, zones, contract.LifecycleReady, published)

	west, east := snapshot.Coverage[0], snapshot.Coverage[1]
	if west.Available != 2 || west.State != contract.CoverageMeeting {
		t.Errorf("west = %+v, want two available and meeting its minimum", west)
	}
	if east.Available != 0 || east.State != contract.CoverageNoneAvailable {
		t.Errorf("east = %+v, want nothing available: its only vehicle is being driven", east)
	}
	if snapshot.Summary.AvailableOutsideAnyZone != 1 {
		t.Errorf("AvailableOutsideAnyZone = %d, want 1", snapshot.Summary.AvailableOutsideAnyZone)
	}
}

func TestCoverageIsBelowMinimumWhileStillPositive(t *testing.T) {
	fleetState := []fleet.Vehicle{reporting("1", contract.StatusFree, inWest, 55, 0)}

	if got := Snapshot(fleetState, zones, contract.LifecycleReady, published).Coverage[0]; got.State != contract.CoverageBelowMinimum {
		t.Errorf("west = %+v, want below its minimum of two", got)
	}
}

// The trap PRODUCT-SPEC §7.2 recorded: a vehicle assigned a job changes its zone's coverage
// without moving a metre, so anything keyed on position changes would be silently wrong.
func TestCoverageRespondsToAStatusChangeWithoutMovement(t *testing.T) {
	parked := reporting("1", contract.StatusFree, inEast, 55, 0)
	if got := Snapshot([]fleet.Vehicle{parked}, zones, contract.LifecycleReady, published).Coverage[1].Available; got != 1 {
		t.Fatalf("east available = %d, want 1", got)
	}

	dispatched := parked
	dispatched.Status = contract.StatusEnRoute
	if got := Snapshot([]fleet.Vehicle{dispatched}, zones, contract.LifecycleReady, published).Coverage[1].Available; got != 0 {
		t.Errorf("east available = %d, want 0: the vehicle is committed even though it has not moved", got)
	}
}

// F8 treats a figure that disagrees with the map as a defect, so the two reconciliations that
// make the summary trustworthy are asserted rather than eyeballed.
func TestSummaryReconcilesWithTheFleet(t *testing.T) {
	fleetState := []fleet.Vehicle{
		reporting("1", contract.StatusFree, inWest, 55, 0),
		reporting("2", contract.StatusFree, inWest, 12, 0),
		reporting("3", contract.StatusFree, betweenZones, 55, 0),
		reporting("4", contract.StatusFree, farAway, 55, 0),
		reporting("5", contract.StatusEnRoute, inEast, 55, 0),
		reporting("6", contract.StatusWithCustomer, farAway, 8, contract.StaleAfter*3),
		{ID: "7", Label: "LV-7"},
		{ID: "8", Label: "LV-8"},
	}
	snapshot := Snapshot(fleetState, zones, contract.LifecycleReady, published)
	summary := snapshot.Summary

	if summary.Total != len(fleetState) {
		t.Errorf("Total = %d, want every registered vehicle (%d)", summary.Total, len(fleetState))
	}
	if got := summary.Free + summary.EnRoute + summary.WithCustomer + summary.AwaitingFirstReport; got != summary.Total {
		t.Errorf("status counts plus awaiting = %d, want Total (%d)", got, summary.Total)
	}
	if got := summary.Total - summary.AwaitingFirstReport; got != len(snapshot.Vehicles) {
		t.Errorf("Total minus awaiting = %d, want the number of markers on the map (%d)", got, len(snapshot.Vehicles))
	}

	inZones := 0
	for _, zone := range snapshot.Coverage {
		inZones += zone.Available
	}
	if got := inZones + summary.AvailableOutsideAnyZone; got != summary.Free {
		t.Errorf("zone availability plus those in no zone = %d, want Free (%d)", got, summary.Free)
	}

	if summary.LowBattery != 2 || summary.Stale != 1 {
		t.Errorf("LowBattery, Stale = %d, %d; want 2, 1 — counted by condition, so a vehicle that is both is in both", summary.LowBattery, summary.Stale)
	}
}

func TestRoutesBelongOnlyToRemotelyDrivenVehicles(t *testing.T) {
	route := func(id string) contract.Route {
		return contract.Route{
			RouteID:     id,
			Geometry:    []contract.Point{{0.1, 0.1}, {0.9, 0.9}},
			Destination: contract.Point{0.9, 0.9},
		}
	}

	enRoute := reporting("1", contract.StatusEnRoute, inWest, 55, 0)
	enRoute.Route = route("route-1")

	// A route still held for a vehicle that has stopped being remotely driven. Unordered
	// cross-topic delivery makes this reachable, and the rule that resolves it is the same one
	// that covers a vehicle EN_ROUTE before its route arrives (ADR-0003 consequences).
	withCustomer := reporting("2", contract.StatusWithCustomer, inEast, 55, 0)
	withCustomer.Route = route("route-2")

	awaitingReport := fleet.Vehicle{ID: "3", Label: "LV-3", Status: contract.StatusEnRoute, Route: route("route-3")}

	fleetState := []fleet.Vehicle{enRoute, withCustomer, awaitingReport}
	snapshot := Snapshot(fleetState, zones, contract.LifecycleReady, published)

	if got := snapshot.Vehicles[0].RouteID; got != "route-1" {
		t.Errorf("RouteID = %q, want route-1", got)
	}
	if got := snapshot.Vehicles[1].RouteID; got != "" {
		t.Errorf("RouteID = %q, want none: a customer-driven vehicle has no route to show", got)
	}

	routes := Routes(fleetState).Routes
	if len(routes) != 1 || routes[0].RouteID != "route-1" {
		t.Errorf("Routes = %+v, want only the remotely-driven vehicle's", routes)
	}
}

func TestARemotelyDrivenVehicleWithNoRouteYetDrawsNone(t *testing.T) {
	fleetState := []fleet.Vehicle{reporting("1", contract.StatusEnRoute, inWest, 55, 0)}
	snapshot := Snapshot(fleetState, zones, contract.LifecycleReady, published)

	if got := snapshot.Vehicles[0].RouteID; got != "" {
		t.Errorf("RouteID = %q, want none", got)
	}
	if got := Routes(fleetState).Routes; len(got) != 0 {
		t.Errorf("Routes = %+v, want none", got)
	}
}
