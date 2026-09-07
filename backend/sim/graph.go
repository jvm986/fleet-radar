package sim

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"

	"fleetradar/contract"
)

// The road network is checked in, because vehicles have to stay on streets: a random walk drifts
// through buildings and the map reads as noise rather than as a fleet, which is what F10 forbids.
// One artefact then serves three purposes — movement, route geometry, and visual plausibility
// against a street basemap (ADR-0007 §7.3).
//
// It is authored as a list of roads rather than a list of intersections. Las Vegas is a grid of
// named arterials, so each street carries a longitude, each avenue a latitude, and both name the
// crossings they run between; an intersection exists where a street and an avenue each claim the
// other. That keeps the file readable as streets, keeps intersections exact rather than matched by
// floating-point coincidence, and makes an authoring mistake a load-time panic.

//go:embed network.json
var networkJSON []byte

// NodeID identifies an intersection, as "street/avenue".
type NodeID string

// Graph is the road network: intersections, and which of them are directly connected.
type Graph struct {
	positions map[NodeID]contract.Point
	adjacent  map[NodeID][]NodeID

	// ids is every intersection in a stable order. Selection walks this rather than a map, so a
	// seeded run makes the same choices every time (ADR-0007 §7.11).
	ids []NodeID

	// inside and outside split the network at the service area boundary. The fleet is placed and
	// dispatched inside it, because that is the ground the operator is responsible for; a vehicle gets
	// beyond it only when a customer drives there, which is the only cause the spec gives
	// (PRODUCT-SPEC §2.5, ADR-0007 §7.12).
	inside  []NodeID
	outside []NodeID
}

// axis is one street or one avenue: where it runs, and between which two crossings.
type axis struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	At   float64 `json:"at"`
	From string  `json:"from"`
	To   string  `json:"to"`
}

func mustLoadGraph(raw []byte) *Graph {
	var network struct {
		Streets []axis `json:"streets"`
		Avenues []axis `json:"avenues"`
	}
	if err := json.Unmarshal(raw, &network); err != nil {
		panic(fmt.Sprintf("sim: network.json is not readable: %v", err))
	}

	streets := mustOrder(network.Streets, "streets run west to east")
	avenues := mustOrder(network.Avenues, "avenues run south to north")

	graph := &Graph{
		positions: make(map[NodeID]contract.Point),
		adjacent:  make(map[NodeID][]NodeID),
	}
	for _, street := range network.Streets {
		for _, avenue := range network.Avenues {
			if !spans(street, avenues, avenue.ID) || !spans(avenue, streets, street.ID) {
				continue
			}
			id := intersection(street, avenue)
			graph.positions[id] = contract.Point{street.At, avenue.At}
			graph.ids = append(graph.ids, id)
		}
	}
	if len(graph.ids) == 0 {
		panic("sim: network.json describes no intersections")
	}

	// Consecutive intersections along each road are connected. Where a road passes a crossing it
	// does not meet, its neighbours join directly, which is what the road doing so means.
	for _, street := range network.Streets {
		var previous NodeID
		for _, avenue := range network.Avenues {
			previous = graph.link(previous, intersection(street, avenue))
		}
	}
	for _, avenue := range network.Avenues {
		var previous NodeID
		for _, street := range network.Streets {
			previous = graph.link(previous, intersection(street, avenue))
		}
	}

	area := contract.ServiceArea()
	for _, id := range graph.ids {
		if contract.Contains(area, graph.positions[id]) {
			graph.inside = append(graph.inside, id)
		} else {
			graph.outside = append(graph.outside, id)
		}
	}
	if len(graph.inside) == 0 || len(graph.outside) == 0 {
		panic("sim: the network must have intersections both inside and outside the service area")
	}
	return graph
}

func intersection(street, avenue axis) NodeID {
	return NodeID(street.ID + "/" + avenue.ID)
}

// link joins previous to next if both exist, and reports which node a following segment should
// start from.
func (g *Graph) link(previous, next NodeID) NodeID {
	if _, ok := g.positions[next]; !ok {
		return previous
	}
	if previous != "" {
		g.adjacent[previous] = append(g.adjacent[previous], next)
		g.adjacent[next] = append(g.adjacent[next], previous)
	}
	return next
}

// mustOrder indexes one set of roads by id. The order is load-bearing, because a road's extent is
// expressed as the crossings it runs between, so an out-of-order file would silently change which
// intersections exist.
func mustOrder(roads []axis, ordering string) map[string]int {
	order := make(map[string]int, len(roads))
	for i, road := range roads {
		if _, repeated := order[road.ID]; repeated {
			panic(fmt.Sprintf("sim: %s is defined twice", road.Name))
		}
		if i > 0 && road.At <= roads[i-1].At {
			panic(fmt.Sprintf("sim: %s is out of order in network.json, where %s", road.Name, ordering))
		}
		order[road.ID] = i
	}
	return order
}

func spans(road axis, crossings map[string]int, target string) bool {
	from, known := crossings[road.From]
	if !known {
		panic(fmt.Sprintf("sim: %s runs from %q, which is not a crossing", road.Name, road.From))
	}
	to, known := crossings[road.To]
	if !known {
		panic(fmt.Sprintf("sim: %s runs to %q, which is not a crossing", road.Name, road.To))
	}
	if from > to {
		panic(fmt.Sprintf("sim: %s runs backwards", road.Name))
	}
	at := crossings[target]
	return from <= at && at <= to
}

func (g *Graph) Position(id NodeID) contract.Point { return g.positions[id] }

// RandomInsideNode is where a vehicle is parked at startup: inside the area it serves. Starting some of
// the fleet beyond the boundary would make out-of-area vehicles observable immediately, but for the wrong
// reason — and a vehicle parked outside the service area at startup reads as a bug rather than as
// information (ADR-0007 §7.7).
func (g *Graph) RandomInsideNode(random *rand.Rand) NodeID {
	return g.inside[random.IntN(len(g.inside))]
}

// Inside and Outside are the two halves of the network. Journeys the scheduler assigns stay inside;
// only a customer's own trip may end beyond the boundary.
func (g *Graph) Inside() []NodeID { return g.inside }

func (g *Graph) Outside() []NodeID { return g.outside }

// Path returns the shortest road path to a destination chosen at random from candidates that lie
// between minMetres and maxMetres away by road. A nil candidate list means anywhere on the
// network. It returns nil when nothing qualifies, which the caller reads as "choose differently"
// rather than as an error.
//
// The returned path excludes the origin and ends at the destination.
func (g *Graph) Path(origin NodeID, candidates []NodeID, minMetres, maxMetres float64, random *rand.Rand) []NodeID {
	distance, previous := g.reachable(origin)
	if candidates == nil {
		candidates = g.ids
	}

	eligible := make([]NodeID, 0, len(candidates))
	for _, id := range candidates {
		// Never the origin. With a minimum of zero it would otherwise qualify, and "the shortest path
		// from here to here" is an empty path — which the caller would read as having nowhere to go, and
		// the vehicle would sit still for the rest of the run.
		if id == origin {
			continue
		}
		if reached, ok := distance[id]; ok && reached >= minMetres && reached <= maxMetres {
			eligible = append(eligible, id)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	var path []NodeID
	for at := eligible[random.IntN(len(eligible))]; at != origin; at = previous[at] {
		path = append(path, at)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// reachable is Dijkstra from one origin, returning the road distance to every intersection it can
// reach and the predecessor on the way there — so one pass serves both choosing a destination and
// drawing the path to it.
//
// It scans for the nearest unvisited node rather than using a heap. The network has under a
// hundred intersections and this runs once per assignment, so the simpler code is the better
// trade; a heap is what changes if either of those stops being true.
func (g *Graph) reachable(origin NodeID) (map[NodeID]float64, map[NodeID]NodeID) {
	distance := map[NodeID]float64{origin: 0}
	previous := make(map[NodeID]NodeID)
	visited := make(map[NodeID]bool, len(g.ids))

	for {
		nearest, found := NodeID(""), math.Inf(1)
		for _, id := range g.ids {
			if reached, ok := distance[id]; ok && !visited[id] && reached < found {
				nearest, found = id, reached
			}
		}
		if nearest == "" {
			return distance, previous
		}
		visited[nearest] = true

		for _, neighbour := range g.adjacent[nearest] {
			through := found + metresBetween(g.positions[nearest], g.positions[neighbour])
			if reached, ok := distance[neighbour]; !ok || through < reached {
				distance[neighbour] = through
				previous[neighbour] = nearest
			}
		}
	}
}

// metresPerDegreeLatitude is close enough anywhere, and longitude is scaled by the cosine of the
// latitude. An equirectangular approximation over a single city is accurate to well within the
// precision anything here needs.
const metresPerDegreeLatitude = 111_320.0

func metresBetween(from, to contract.Point) float64 {
	latitude := (from.Lat() + to.Lat()) / 2 * math.Pi / 180
	east := (to.Lng() - from.Lng()) * metresPerDegreeLatitude * math.Cos(latitude)
	north := (to.Lat() - from.Lat()) * metresPerDegreeLatitude
	return math.Hypot(east, north)
}

// bearingTo is degrees clockwise from north, which is what the marker is rotated by.
func bearingTo(from, to contract.Point) float64 {
	latitude := (from.Lat() + to.Lat()) / 2 * math.Pi / 180
	east := (to.Lng() - from.Lng()) * math.Cos(latitude)
	north := to.Lat() - from.Lat()

	degrees := math.Atan2(east, north) * 180 / math.Pi
	if degrees < 0 {
		degrees += 360
	}
	return degrees
}

// towards moves from one position towards another by a distance, and reports whether it arrived.
func towards(from, to contract.Point, metres float64) (contract.Point, bool) {
	remaining := metresBetween(from, to)
	if remaining <= metres {
		return to, true
	}
	fraction := metres / remaining
	return contract.Point{
		from.Lng() + (to.Lng()-from.Lng())*fraction,
		from.Lat() + (to.Lat()-from.Lat())*fraction,
	}, false
}
