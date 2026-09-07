package sim

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"

	"fleetradar/contract"
)

// The network is parsed once for the whole file. It is two megabytes of real geometry and nothing
// here mutates it, so a copy per test would only make the suite slower.
var network = sync.OnceValue(func() *Graph { return mustLoadGraph(networkJSON) })

// sampled is how many intersections the tests that cannot afford the whole network check. Twenty-two
// thousand routings is minutes; a seeded sample of this size covers every part of the map and runs in
// about a second.
const sampled = 200

// A vehicle that drives somewhere it cannot leave sits still for the rest of the run with nothing
// saying why. The authored grid was connected by construction; real data is not, so roadgen prunes
// the network to its largest mutually-reachable set and this is the check that it did.
//
// Two traversals rather than one per intersection: everything is reachable from one node, and that
// node is reachable from everything, which together is exactly the property wanted. One-way roads are
// why both directions have to be checked — a dead end you can drive into is reachable from the origin
// while not reaching it.
func TestEveryIntersectionIsReachableFromEveryOther(t *testing.T) {
	graph := network()

	into := make([][]NodeID, graph.Intersections())
	for node := range NodeID(graph.Intersections()) {
		for _, out := range graph.leaving[node] {
			into[out.to] = append(into[out.to], node)
		}
	}

	outward := func(node NodeID) []NodeID {
		neighbours := make([]NodeID, 0, len(graph.leaving[node]))
		for _, out := range graph.leaving[node] {
			neighbours = append(neighbours, out.to)
		}
		return neighbours
	}
	inward := func(node NodeID) []NodeID { return into[node] }

	if reached := spread(graph, 0, outward); reached != graph.Intersections() {
		t.Errorf("only %d of %d intersections can be driven to from the first one", reached, graph.Intersections())
	}
	if reached := spread(graph, 0, inward); reached != graph.Intersections() {
		t.Errorf("only %d of %d intersections can drive to the first one", reached, graph.Intersections())
	}
}

// spread counts what a flood fill from one intersection reaches, following whichever direction it is
// given.
func spread(graph *Graph, from NodeID, neighbours func(NodeID) []NodeID) int {
	seen := make([]bool, graph.Intersections())
	seen[from] = true

	count, queue := 1, []NodeID{from}
	for len(queue) > 0 {
		at := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		for _, next := range neighbours(at) {
			if !seen[next] {
				seen[next] = true
				count++
				queue = append(queue, next)
			}
		}
	}
	return count
}

// The network must extend past the boundary, or a customer-driven vehicle can never leave the
// service area and a behaviour the spec requires cannot be shown (ADR-0007 §7.12).
func TestTheNetworkExtendsBeyondTheServiceArea(t *testing.T) {
	graph := network()

	if len(graph.Outside()) == 0 {
		t.Fatal("every intersection is inside the service area, so no vehicle can ever leave it")
	}

	random := rand.New(rand.NewPCG(1, 2))
	origin := graph.RandomInsideNode(random)
	if route := graph.Path(origin, graph.Outside(), 0, math.Inf(1), random); len(route) == 0 {
		t.Errorf("no route from %d to anywhere outside the service area", origin)
	}
}

// A zone with no intersections in it can never hold a vehicle, so its coverage would read as
// nothing available for the entire run — a feature that looks broken rather than untested.
func TestEveryZoneContainsIntersections(t *testing.T) {
	graph := network()

	for _, zone := range contract.Zones() {
		found := 0
		for node := range NodeID(graph.Intersections()) {
			if contract.Contains(zone.Boundary, graph.Position(node)) {
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
// reviewer sees several complete. Measured along the road geometry, because that is the distance the
// vehicle actually drives — measuring between intersections would understate every route that bends.
func TestAJourneyStaysWithinItsBounds(t *testing.T) {
	graph := network()
	random := rand.New(rand.NewPCG(7, 11))

	for range sampled {
		origin := graph.RandomInsideNode(random)
		route := graph.Path(origin, graph.Inside(), TripMinMetres, TripMaxMetres, random)
		if len(route) == 0 {
			t.Fatalf("no journey available from %d", origin)
		}

		travelled := driven(graph, graph.Position(origin), route)
		if travelled < TripMinMetres || travelled > TripMaxMetres {
			t.Errorf("journey from %d is %.0f metres, want between %.0f and %.0f",
				origin, travelled, TripMinMetres, TripMaxMetres)
		}
	}
}

// A one-way road may only be left from the end it starts at. Getting this backwards would put
// vehicles driving the wrong way up divided arterials, which is the kind of detail that makes an
// otherwise plausible map read as broken.
func TestAOneWayRoadIsOnlyDrivenInItsDirection(t *testing.T) {
	graph := network()

	oneway := 0
	for index, r := range graph.roads {
		forwards, backwards := false, false
		for _, out := range graph.leaving[r.a] {
			forwards = forwards || out.road == int32(index)
		}
		for _, out := range graph.leaving[r.b] {
			backwards = backwards || out.road == int32(index)
		}

		if !forwards {
			t.Fatalf("road %d cannot be driven from either of its ends", index)
		}
		if !backwards {
			oneway++
		}
	}

	t.Logf("%d of %d roads are one-way", oneway, len(graph.roads))
	if oneway == 0 {
		t.Error("no road is one-way, so the extract has lost its direction tags")
	}
}

// A route carries the shape of the roads it runs along, and carries it the way round the vehicle is
// driving. A road traversed against its stored direction with its shape left forwards would send the
// vehicle back down the street and then forwards again — visible as a zigzag, and silent in any test
// that only checks the intersections.
func TestARouteCarriesRoadShapeInTheDirectionOfTravel(t *testing.T) {
	graph := network()

	// A two-way road with shape to it, which is what makes the direction observable at all.
	var bent int32 = -1
	for index, r := range graph.roads {
		if len(r.via) < 2 {
			continue
		}
		for _, out := range graph.leaving[r.b] {
			if out.road == int32(index) {
				bent = int32(index)
			}
		}
		if bent >= 0 {
			break
		}
	}
	if bent < 0 {
		t.Skip("the network has no two-way road with shape to it")
	}

	road := graph.roads[bent]
	forwards := graph.shape(link{road: bent, to: road.b, forward: true})
	backwards := graph.shape(link{road: bent, to: road.a, forward: false})

	if len(forwards) != len(backwards) {
		t.Fatalf("road %d has %d shape points one way and %d the other", bent, len(forwards), len(backwards))
	}
	for i := range forwards {
		if forwards[i] != backwards[len(backwards)-1-i] {
			t.Fatalf("road %d driven backwards is not its shape reversed", bent)
		}
	}

	// And the shape has to reach the route, or none of the above matters to the vehicle.
	random := rand.New(rand.NewPCG(13, 17))
	for range sampled {
		origin := graph.RandomInsideNode(random)
		for _, step := range graph.Path(origin, graph.Inside(), TripMinMetres, TripMaxMetres, random) {
			if len(step.Via) > 0 {
				return
			}
		}
	}
	t.Errorf("no route in %d journeys carried any road shape, so vehicles are cutting between intersections", sampled)
}

// driven is how far a vehicle travels along a route, following each road's shape.
func driven(graph *Graph, from contract.Point, route []Step) float64 {
	travelled, at := 0.0, from
	for _, step := range route {
		for _, next := range step.Via {
			travelled += metresBetween(at, next)
			at = next
		}
		next := graph.Position(step.Node)
		travelled += metresBetween(at, next)
		at = next
	}
	return travelled
}
