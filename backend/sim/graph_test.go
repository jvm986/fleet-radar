package sim

import (
	"math"
	"math/rand/v2"
	"testing"

	"fleetradar/contract"
)

// A disconnected network would leave part of the fleet unable to reach anywhere, and the vehicles
// stranded there would sit still for the whole run without anything saying why.
func TestEveryIntersectionIsReachableFromEveryOther(t *testing.T) {
	graph := mustLoadGraph(networkJSON)

	for _, origin := range graph.ids {
		distance, _ := graph.reachable(origin)
		if len(distance) != len(graph.ids) {
			t.Fatalf("from %s only %d of %d intersections are reachable", origin, len(distance), len(graph.ids))
		}
	}
}

// The network must extend past the boundary, or a customer-driven vehicle can never leave the
// service area and a behaviour the spec requires cannot be shown. Easy to author, easy to forget
// (ADR-0007 §7.12).
func TestTheNetworkExtendsBeyondTheServiceArea(t *testing.T) {
	graph := mustLoadGraph(networkJSON)

	if len(graph.Outside()) == 0 {
		t.Fatal("every intersection is inside the service area, so no vehicle can ever leave it")
	}

	origin := graph.ids[0]
	if path := graph.Path(origin, graph.Outside(), 0, math.Inf(1), rand.New(rand.NewPCG(1, 2))); len(path) == 0 {
		t.Errorf("no route from %s to anywhere outside the service area", origin)
	}
}

// A zone with no intersections in it can never hold a vehicle, so its coverage would read as
// nothing available for the entire run — a feature that looks broken rather than untested.
func TestEveryZoneContainsIntersections(t *testing.T) {
	graph := mustLoadGraph(networkJSON)

	for _, zone := range contract.Zones() {
		found := 0
		for _, id := range graph.ids {
			if contract.Contains(zone.Boundary, graph.Position(id)) {
				found++
			}
		}
		t.Logf("%s: %d intersections, minimum %d available", zone.Name, found, zone.Minimum)
		if found == 0 {
			t.Errorf("zone %q contains no intersections", zone.ID)
		}
	}
}

// Journeys have to be journeys: long enough to be a trip across a city, short enough that a
// reviewer sees several complete.
func TestAJourneyStaysWithinItsBounds(t *testing.T) {
	graph := mustLoadGraph(networkJSON)
	random := rand.New(rand.NewPCG(7, 11))

	for _, origin := range graph.ids {
		path := graph.Path(origin, nil, TripMinMetres, TripMaxMetres, random)
		if len(path) == 0 {
			t.Fatalf("no journey available from %s", origin)
		}

		travelled, at := 0.0, graph.Position(origin)
		for _, id := range path {
			next := graph.Position(id)
			travelled += metresBetween(at, next)
			at = next
		}
		if travelled < TripMinMetres || travelled > TripMaxMetres {
			t.Errorf("journey from %s is %.0f metres, want between %.0f and %.0f", origin, travelled, TripMinMetres, TripMaxMetres)
		}
	}
}
