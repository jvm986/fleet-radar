package sim

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"

	"fleetradar/contract"
)

// The road network is real OpenStreetMap geometry, extracted by cmd/roadgen and checked in. Vehicles
// have to stay on streets — a random walk drifts through buildings and the map reads as noise rather
// than as a fleet, which is what F10 forbids — and one artefact then serves three purposes: movement,
// route geometry, and visual plausibility against a street basemap (ADR-0007 §7.3).
//
// It replaces a hand-authored grid of eleven streets and ten avenues. ADR-0007 rejected "a real
// routing engine" and was right to: OSRM is a Docker dependency and the "Go, Node and make"
// prerequisite is worth more than turn costs. But rejecting the engine did not require inventing the
// data. The routing is still ours — Dijkstra, below — over geometry that is real, so the fleet drives
// the actual curve of Las Vegas Boulevard and does not cross the airport.
//
// What the authored grid got for free and this does not: it was connected by construction, and an
// authoring mistake was a load-time panic. Real data is neither, so the invariants moved into
// roadgen, which keeps only the intersections that can all reach one another and refuses to emit a
// network the product cannot be demonstrated on.

//go:embed network.json
var networkJSON []byte

// NodeID identifies an intersection by its position in the artefact, which is what lets the graph be
// slices rather than maps — the difference matters because routing touches every node.
type NodeID int32

// Step is one road driven: the shape to follow, and the intersection at the end of it. Via is the
// road's own geometry, so a vehicle rounds a bend rather than cutting across it, and it is consumed
// as the vehicle passes each point.
type Step struct {
	Node NodeID
	Via  []contract.Point
}

// Graph is the road network: where the intersections are, and which roads connect them.
type Graph struct {
	positions []contract.Point
	names     []string
	roads     []road
	// leaving is every road that can be driven out of an intersection, which is not the same as every
	// road that touches it: a one-way street is only in the list of the end you may leave from.
	leaving [][]link

	// inside and outside split the network at the service area boundary. The fleet is placed and
	// dispatched inside it, because that is the ground the operator is responsible for; a vehicle gets
	// beyond it only when a customer drives there, which is the only cause the spec gives
	// (PRODUCT-SPEC §2.5, ADR-0007 §7.12).
	inside  []NodeID
	outside []NodeID
}

// road is one stretch of street between two intersections, stored once however many directions it
// may be driven in.
type road struct {
	a, b   NodeID
	name   int32
	via    []contract.Point
	metres float64
}

// link is one road as seen from one of its ends: which road, where it comes out, and whether that is
// along the road's stored direction or against it — which is what decides whether its shape is
// followed forwards or backwards.
type link struct {
	road    int32
	to      NodeID
	forward bool
}

// noLink marks an intersection routing has not reached.
const noLink = int32(-1)

func mustLoadGraph(raw []byte) *Graph {
	var network struct {
		Names []string         `json:"names"`
		Nodes []contract.Point `json:"nodes"`
		Roads []struct {
			A      NodeID           `json:"a"`
			B      NodeID           `json:"b"`
			Name   int32            `json:"name"`
			Oneway bool             `json:"oneway"`
			Via    []contract.Point `json:"via"`
		} `json:"roads"`
	}
	if err := json.Unmarshal(raw, &network); err != nil {
		panic(fmt.Sprintf("sim: network.json is not readable: %v", err))
	}
	if len(network.Nodes) == 0 {
		panic("sim: network.json describes no intersections")
	}
	if len(network.Roads) == 0 {
		panic("sim: network.json describes no roads")
	}

	graph := &Graph{
		positions: network.Nodes,
		names:     network.Names,
		roads:     make([]road, 0, len(network.Roads)),
		leaving:   make([][]link, len(network.Nodes)),
	}

	for _, described := range network.Roads {
		// Checked rather than trusted, because an index past the end of the node table is the one
		// authoring mistake a generated file can still contain, and a silent one: it would panic later,
		// during a journey, rather than here.
		if !graph.holds(described.A) || !graph.holds(described.B) {
			panic(fmt.Sprintf("sim: a road runs between %d and %d, and the network has %d intersections",
				described.A, described.B, len(graph.positions)))
		}
		if described.Name < 0 || int(described.Name) >= len(graph.names) {
			panic(fmt.Sprintf("sim: a road names street %d, and the network names %d", described.Name, len(graph.names)))
		}

		index := int32(len(graph.roads))
		graph.roads = append(graph.roads, road{
			a: described.A, b: described.B,
			name:   described.Name,
			via:    described.Via,
			metres: graph.length(described.A, described.B, described.Via),
		})

		graph.leaving[described.A] = append(graph.leaving[described.A], link{road: index, to: described.B, forward: true})
		if !described.Oneway {
			graph.leaving[described.B] = append(graph.leaving[described.B], link{road: index, to: described.A, forward: false})
		}
	}

	area := contract.ServiceArea()
	for node := range NodeID(len(graph.positions)) {
		if contract.Contains(area, graph.positions[node]) {
			graph.inside = append(graph.inside, node)
		} else {
			graph.outside = append(graph.outside, node)
		}
	}
	if len(graph.inside) == 0 || len(graph.outside) == 0 {
		panic("sim: the network must have intersections both inside and outside the service area")
	}
	return graph
}

func (g *Graph) holds(node NodeID) bool { return node >= 0 && int(node) < len(g.positions) }

// length is the road's driven distance, along its shape rather than straight between its ends.
func (g *Graph) length(a, b NodeID, via []contract.Point) float64 {
	metres, at := 0.0, g.positions[a]
	for _, next := range via {
		metres += metresBetween(at, next)
		at = next
	}
	return metres + metresBetween(at, g.positions[b])
}

func (g *Graph) Position(node NodeID) contract.Point { return g.positions[node] }

// Intersections is how many the network has, which is reported at startup because the size of the
// world is the first thing to check when movement looks wrong.
func (g *Graph) Intersections() int { return len(g.positions) }

// Roads is how many stretches of street the network holds.
func (g *Graph) Roads() int { return len(g.roads) }

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

// Path returns the shortest road route to a destination chosen at random from candidates that lie
// between minMetres and maxMetres away by road. A nil candidate list means anywhere on the network.
// It returns nil when nothing qualifies, which the caller reads as "choose differently" rather than
// as an error.
//
// The route excludes the origin and ends at the destination.
func (g *Graph) Path(origin NodeID, candidates []NodeID, minMetres, maxMetres float64, random *rand.Rand) []Step {
	distance, arrival := g.reachable(origin, maxMetres)
	if candidates == nil {
		candidates = g.all()
	}

	eligible := make([]NodeID, 0, 64)
	for _, node := range candidates {
		// Never the origin. With a minimum of zero it would otherwise qualify, and "the shortest route
		// from here to here" is an empty one — which the caller would read as having nowhere to go, and
		// the vehicle would sit still for the rest of the run.
		if node == origin {
			continue
		}
		if reached := distance[node]; reached >= minMetres && reached <= maxMetres {
			eligible = append(eligible, node)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	var route []Step
	for at := eligible[random.IntN(len(eligible))]; at != origin; {
		used := g.roads[arrival[at].road]
		route = append(route, Step{Node: at, Via: g.shape(arrival[at])})
		if arrival[at].forward {
			at = used.a
		} else {
			at = used.b
		}
	}
	for i, j := 0, len(route)-1; i < j; i, j = i+1, j-1 {
		route[i], route[j] = route[j], route[i]
	}
	return route
}

// all is every intersection, for the callers that will take anywhere at all.
func (g *Graph) all() []NodeID {
	every := make([]NodeID, len(g.positions))
	for node := range NodeID(len(g.positions)) {
		every[node] = node
	}
	return every
}

// shape is a road's geometry in the direction it is being driven.
func (g *Graph) shape(used link) []contract.Point {
	via := g.roads[used.road].via
	if used.forward || len(via) == 0 {
		return via
	}

	// Reversed rather than stored twice: most roads carry no shape at all, because the extract splits
	// them at every junction, so the copy is rare and a second copy of every road's geometry is not.
	backwards := make([]contract.Point, len(via))
	for i, at := range via {
		backwards[len(via)-1-i] = at
	}
	return backwards
}

// reachable is Dijkstra from one origin, returning the road distance to every intersection within
// limit and the road that arrives there — so one pass serves both choosing a destination and drawing
// the route to it.
//
// It stops once the nearest remaining intersection is beyond the limit, which is what makes this
// affordable on a real network: journeys are a few kilometres, and a few kilometres is a couple of
// thousand intersections out of twenty-two thousand. The authored grid used a linear scan for the
// nearest node instead of a heap, on the grounds that under a hundred intersections made the simpler
// code the better trade. That reasoning was sound and its premise is gone.
func (g *Graph) reachable(origin NodeID, limit float64) ([]float64, []link) {
	distance := make([]float64, len(g.positions))
	for node := range distance {
		distance[node] = math.Inf(1)
	}
	arrival := make([]link, len(g.positions))
	for node := range arrival {
		arrival[node].road = noLink
	}
	settled := make([]bool, len(g.positions))

	distance[origin] = 0
	queue := &frontier{}
	queue.push(origin, 0)

	for queue.len() > 0 {
		nearest, reached := queue.pop()
		if settled[nearest] {
			continue
		}
		if reached > limit {
			break
		}
		settled[nearest] = true

		for _, out := range g.leaving[nearest] {
			through := reached + g.roads[out.road].metres
			if through < distance[out.to] {
				distance[out.to] = through
				arrival[out.to] = out
				queue.push(out.to, through)
			}
		}
	}
	return distance, arrival
}

// frontier is the binary heap Dijkstra pops from. It is written out rather than taken from
// container/heap because the interface costs an allocation per entry and a type assertion per
// comparison, and this is the hot loop of the simulation.
//
// Entries are never removed on improvement, only added; a node popped after it has been settled is
// skipped. That trades a slightly larger heap for not having to find and sift an existing entry.
type frontier struct {
	nodes     []NodeID
	distances []float64
}

func (f *frontier) len() int { return len(f.nodes) }

func (f *frontier) push(node NodeID, distance float64) {
	f.nodes = append(f.nodes, node)
	f.distances = append(f.distances, distance)

	for child := len(f.nodes) - 1; child > 0; {
		parent := (child - 1) / 2
		if f.distances[parent] <= f.distances[child] {
			break
		}
		f.swap(parent, child)
		child = parent
	}
}

func (f *frontier) pop() (NodeID, float64) {
	node, distance := f.nodes[0], f.distances[0]

	last := len(f.nodes) - 1
	f.swap(0, last)
	f.nodes = f.nodes[:last]
	f.distances = f.distances[:last]

	for parent := 0; ; {
		smallest := parent
		for _, child := range [2]int{2*parent + 1, 2*parent + 2} {
			if child < len(f.nodes) && f.distances[child] < f.distances[smallest] {
				smallest = child
			}
		}
		if smallest == parent {
			break
		}
		f.swap(parent, smallest)
		parent = smallest
	}
	return node, distance
}

func (f *frontier) swap(i, j int) {
	f.nodes[i], f.nodes[j] = f.nodes[j], f.nodes[i]
	f.distances[i], f.distances[j] = f.distances[j], f.distances[i]
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
