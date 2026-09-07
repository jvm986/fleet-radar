// Command roadgen builds the simulator's road network from OpenStreetMap.
//
// ADR-0007 §7.3 chose a hand-authored graph over a real routing engine, and it was right about the
// engine: OSRM or Valhalla is a Docker dependency, and the "Go, Node and make" prerequisite is worth
// more than turn costs. But it conflated two things. Rejecting a routing *engine* does not require
// inventing the *data*. This command takes the real geometry and leaves the routing to us.
//
// It runs by hand, not as part of the build. The output is committed, so a clone needs no network and
// the graph cannot change under a reviewer mid-session; regenerating is a deliberate act that produces
// a reviewable diff. OpenStreetMap also changes daily, so a build-time fetch would make two clones of
// the same commit disagree about the world.
//
// Usage, from backend/:
//
//	go run ./cmd/roadgen > sim/network.json
//	go run ./cmd/roadgen -cache /tmp/overpass.json > sim/network.json
//
// The cache holds the raw Overpass response, which is around 20 MB and takes a few seconds to fetch.
// It exists so that changing the extraction and re-running does not re-query a free public service
// every time.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"fleetradar/contract"
)

const (
	endpoint = "https://overpass-api.de/api/interpreter"

	// agent identifies this generator to Overpass.
	agent = "fleetradar-roadgen/1.0 (simulator road network; contact via repository)"

	// The extract is the service area with room around it in every direction, because the network has
	// to continue past the boundary: a customer-driven vehicle leaves it, and if the data stopped at the
	// edge the road would too, so the vehicle would arrive at a cliff rather than at a destination
	// (ADR-0007 §7.12).
	south, west, north, east = 36.0100, -115.3300, 36.3200, -115.0000

	// Down to tertiary. Primary and secondary alone are the arterials, and a fleet confined to them
	// drives a handful of long straight lines that look no more like a city than the authored grid did.
	// Residential streets are the other extreme: they quadruple the artefact for movement a viewer at
	// working zoom cannot distinguish. The link roads are not optional at any level — they are the slip
	// roads that connect the classes to each other, and without them the network fragments into
	// per-class islands.
	classes = "^(primary|primary_link|secondary|secondary_link|tertiary|tertiary_link)$"

	// simplifyMetres discards geometry no one can see. A vehicle marker is a few metres across at the
	// zoom the operator works at, so a bend flattened by less than that is a bend they were never shown.
	// It is deliberately far below the ~15 metres a vehicle covers between reports: the geometry still
	// has to be finer than the movement along it, or the simplification becomes visible as a vehicle
	// cutting a corner.
	simplifyMetres = 2.5

	// Five decimal places is about 1.1 metres of longitude here. Beyond that the file grows to record
	// differences smaller than the simplification tolerance already throws away.
	places = 5

	// metresPerDegreeLatitude matches sim's own approximation. An equirectangular projection over one
	// city is accurate to well within what any of this needs.
	metresPerDegreeLatitude = 111_320.0
)

func main() {
	cache := flag.String("cache", "", "path to a cached raw Overpass response; written if absent, read if present")
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("roadgen: ")

	raw := load(*cache)

	ways := parse(raw)
	log.Printf("%d ways in the extract", len(ways))

	network := build(ways)
	network.validate()
	network.write(os.Stdout)
}

// load reads the cached response or fetches one, so that iterating on the extraction does not
// re-query a free public service each time.
func load(cache string) []byte {
	if cache != "" {
		if raw, err := os.ReadFile(cache); err == nil {
			log.Printf("read %.1f MB from %s", float64(len(raw))/(1<<20), cache)
			return raw
		}
	}

	raw := fetch()
	if cache != "" {
		if err := os.WriteFile(cache, raw, 0o644); err != nil {
			log.Printf("could not write the cache: %v", err)
		}
	}
	return raw
}

func fetch() []byte {
	log.Printf("querying %s", endpoint)
	started := time.Now()

	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(url.Values{"data": {query()}}.Encode()))
	if err != nil {
		log.Fatalf("the Overpass request could not be built: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Named, because Overpass is a free service that answers 406 to Go's default agent string, and
	// because a service being asked for 20 MB is owed an identity it can rate-limit or contact.
	request.Header.Set("User-Agent", agent)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatalf("the Overpass query failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Fatalf("Overpass answered %s. 429 is rate limiting, 504 is a timed-out query, and 400 is a syntax error in it", response.Status)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatalf("the Overpass response could not be read: %v", err)
	}

	log.Printf("fetched %.1f MB in %s", float64(len(raw))/(1<<20), time.Since(started).Round(time.Millisecond))
	return raw
}

func query() string {
	// out body geom gives both the node ids and the coordinates. The ids are what make an intersection
	// exact: two roads meet where they share a node, which is a fact in the data rather than a
	// floating-point coincidence between two coordinates that nearly match.
	return fmt.Sprintf("[out:json][timeout:180];\nway[\"highway\"~%q](%.4f,%.4f,%.4f,%.4f);\nout body geom;",
		classes, south, west, north, east)
}

// way is one OpenStreetMap way, reduced to what the network needs.
type way struct {
	nodes  []int64
	points []contract.Point
	name   string
	oneway bool
}

func parse(raw []byte) []way {
	var response struct {
		Elements []struct {
			Type     string            `json:"type"`
			Nodes    []int64           `json:"nodes"`
			Geometry []*coordinate     `json:"geometry"`
			Tags     map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		log.Fatalf("the Overpass response is not readable: %v", err)
	}

	var ways []way
	for _, element := range response.Elements {
		if element.Type != "way" || len(element.Nodes) != len(element.Geometry) {
			continue
		}

		// A way whose ends lie outside the queried box comes back with those nodes' coordinates
		// missing, so a run of consecutive known points is the most of the way that can be used. Each
		// run becomes its own way, which loses the connection across the gap — correctly, because we
		// do not know where the road went.
		start := 0
		for i := 0; i <= len(element.Nodes); i++ {
			if i < len(element.Nodes) && element.Geometry[i] != nil {
				continue
			}
			if i-start >= 2 {
				stretch := way{
					nodes:  element.Nodes[start:i],
					points: points(element.Geometry[start:i]),
					name:   element.Tags["name"],
					oneway: directed(element.Tags),
				}
				// A way tagged as running against its own node order is stored flipped, so that
				// everything downstream can assume a road is driven from a to b.
				if element.Tags["oneway"] == "-1" {
					stretch.reverse()
				}
				ways = append(ways, stretch)
			}
			start = i + 1
		}
	}
	if len(ways) == 0 {
		log.Fatal("the extract contains no usable ways")
	}
	return ways
}

type coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func points(coordinates []*coordinate) []contract.Point {
	converted := make([]contract.Point, len(coordinates))
	for i, at := range coordinates {
		converted[i] = round(contract.Point{at.Lon, at.Lat})
	}
	return converted
}

// reverse flips a way end to end, which is how a road tagged as running backwards is normalised.
func (w *way) reverse() {
	for i, j := 0, len(w.nodes)-1; i < j; i, j = i+1, j-1 {
		w.nodes[i], w.nodes[j] = w.nodes[j], w.nodes[i]
		w.points[i], w.points[j] = w.points[j], w.points[i]
	}
}

// directed reports whether a way may only be driven in the order its nodes are listed. Getting this
// wrong is worse than ignoring it: a vehicle driving the wrong way up a divided arterial is exactly
// the kind of detail that makes a plausible map read as broken.
func directed(tags map[string]string) bool {
	switch tags["oneway"] {
	case "yes", "true", "1", "-1":
		return true
	}
	// A roundabout is one-way whether or not anyone tagged it as such.
	return tags["junction"] == "roundabout"
}

func round(at contract.Point) contract.Point {
	scale := math.Pow(10, places)
	return contract.Point{
		math.Round(at.Lng()*scale) / scale,
		math.Round(at.Lat()*scale) / scale,
	}
}

// road is one stretch of street between two intersections, which is the unit the simulator drives
// along. via is the shape in between, so a vehicle follows the road's actual curve rather than the
// straight line between its ends.
type road struct {
	a, b   int32
	name   int32
	oneway bool
	via    []contract.Point
}

// network is the artefact: a string table for names, the intersections, and the roads between them.
type network struct {
	names     []string
	positions []contract.Point
	roads     []road
}

func build(ways []way) *network {
	// An intersection is a node two roads share, plus every way's own ends — a road that stops in the
	// middle of nowhere still stops somewhere, and its end has to be a node or the road cannot be
	// represented at all.
	shared := make(map[int64]int, 1<<16)
	for _, w := range ways {
		for _, node := range w.nodes {
			shared[node]++
		}
		shared[w.nodes[0]]++
		shared[w.nodes[len(w.nodes)-1]]++
	}

	built := &network{}
	names := map[string]int32{}
	indices := map[int64]int32{}

	// Node indices are handed out in the order the ways are listed, and names in the order they are
	// first seen, so the same extract always produces byte-identical output.
	intern := func(node int64, at contract.Point) int32 {
		if index, seen := indices[node]; seen {
			return index
		}
		index := int32(len(built.positions))
		indices[node] = index
		built.positions = append(built.positions, at)
		return index
	}
	name := func(street string) int32 {
		if index, seen := names[street]; seen {
			return index
		}
		index := int32(len(built.names))
		names[street] = index
		built.names = append(built.names, street)
		return index
	}
	name("") // Index zero is the unnamed road, so an absent name needs no special case.

	for _, w := range ways {
		var junctions []int
		for i, node := range w.nodes {
			if shared[node] >= 2 {
				junctions = append(junctions, i)
			}
		}

		for segment := 0; segment+1 < len(junctions); segment++ {
			from, to := junctions[segment], junctions[segment+1]
			a := intern(w.nodes[from], w.points[from])
			b := intern(w.nodes[to], w.points[to])
			if a == b {
				// A closed loop back to the same intersection. It leads nowhere the vehicle is not
				// already, and it makes the road's two ends indistinguishable.
				continue
			}

			built.roads = append(built.roads, road{
				a: a, b: b,
				name:   name(w.name),
				oneway: w.oneway,
				via:    simplify(w.points[from : to+1]),
			})
		}
	}

	log.Printf("%d intersections and %d roads before pruning", len(built.positions), len(built.roads))
	return built.strongest()
}

// simplify drops the shape points that no one can see, and returns the interior of the run — the
// endpoints are the intersections, which are stored separately.
func simplify(run []contract.Point) []contract.Point {
	kept := douglasPeucker(run, simplifyMetres)
	if len(kept) <= 2 {
		return nil
	}
	return kept[1 : len(kept)-1]
}

func douglasPeucker(run []contract.Point, tolerance float64) []contract.Point {
	// Copied rather than returned as-is, because the caller appends onto what comes back and the input
	// is a window onto the way's own geometry.
	if len(run) < 3 {
		return append([]contract.Point(nil), run...)
	}

	first, last := run[0], run[len(run)-1]
	worst, at := 0.0, 0
	for i := 1; i < len(run)-1; i++ {
		if deviation := offLine(first, last, run[i]); deviation > worst {
			worst, at = deviation, i
		}
	}
	if worst <= tolerance {
		return []contract.Point{first, last}
	}

	before := douglasPeucker(run[:at+1], tolerance)
	after := douglasPeucker(run[at:], tolerance)
	return append(before[:len(before)-1], after...)
}

// offLine is how far a point sits from the straight line between two others, in metres.
func offLine(from, to, at contract.Point) float64 {
	scale := math.Cos((from.Lat() + to.Lat()) / 2 * math.Pi / 180)
	east := func(p contract.Point) float64 { return p.Lng() * metresPerDegreeLatitude * scale }
	north := func(p contract.Point) float64 { return p.Lat() * metresPerDegreeLatitude }

	dx, dy := east(to)-east(from), north(to)-north(from)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return math.Hypot(east(at)-east(from), north(at)-north(from))
	}
	return math.Abs(dy*(east(at)-east(from))-dx*(north(at)-north(from))) / length
}

// strongest keeps only the largest set of intersections that can all reach one another, which is the
// one property the simulator cannot work without. A bbox cuts roads mid-street and one-way tags make
// reachability directional, so the raw extract contains dead ends a vehicle can drive into and never
// leave — and a stranded vehicle sits still for the rest of the run with nothing saying why. The
// authored graph got this for free by being a connected grid; real data has to be pruned into it.
func (n *network) strongest() *network {
	forward, backward := make([][]int32, len(n.positions)), make([][]int32, len(n.positions))
	for _, r := range n.roads {
		forward[r.a] = append(forward[r.a], r.b)
		backward[r.b] = append(backward[r.b], r.a)
		if !r.oneway {
			forward[r.b] = append(forward[r.b], r.a)
			backward[r.a] = append(backward[r.a], r.b)
		}
	}

	// Kosaraju: one pass over the graph to order nodes by when they finish, then a pass over the
	// reverse graph in that order, where each tree is one strongly connected component.
	order := make([]int32, 0, len(n.positions))
	seen := make([]bool, len(n.positions))
	for node := range int32(len(n.positions)) {
		if seen[node] {
			continue
		}
		// Iterative, because a component here is tens of thousands of nodes deep and the recursion
		// would be a stack the size of the network.
		stack := []int32{node}
		progress := map[int32]int{}
		seen[node] = true
		for len(stack) > 0 {
			at := stack[len(stack)-1]
			if progress[at] < len(forward[at]) {
				next := forward[at][progress[at]]
				progress[at]++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, next)
				}
				continue
			}
			order = append(order, at)
			stack = stack[:len(stack)-1]
		}
	}

	component := make([]int32, len(n.positions))
	for i := range component {
		component[i] = -1
	}
	largest, size := int32(-1), 0
	for i := len(order) - 1; i >= 0; i-- {
		root := order[i]
		if component[root] != -1 {
			continue
		}
		members := 0
		stack := []int32{root}
		component[root] = root
		for len(stack) > 0 {
			at := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			members++
			for _, next := range backward[at] {
				if component[next] == -1 {
					component[next] = root
					stack = append(stack, next)
				}
			}
		}
		if members > size {
			largest, size = root, members
		}
	}

	// Re-index, so the artefact has no gaps and a node's index is its position in the file.
	kept := &network{names: n.names}
	renumbered := make([]int32, len(n.positions))
	for i := range renumbered {
		renumbered[i] = -1
	}
	for node := range int32(len(n.positions)) {
		if component[node] != largest {
			continue
		}
		renumbered[node] = int32(len(kept.positions))
		kept.positions = append(kept.positions, n.positions[node])
	}
	for _, r := range n.roads {
		if renumbered[r.a] == -1 || renumbered[r.b] == -1 {
			continue
		}
		r.a, r.b = renumbered[r.a], renumbered[r.b]
		kept.roads = append(kept.roads, r)
	}

	log.Printf("kept %d intersections and %d roads, discarding %d intersections not mutually reachable",
		len(kept.positions), len(kept.roads), len(n.positions)-len(kept.positions))
	return kept.compact()
}

// compact drops the names no surviving road uses, so pruning does not leave a string table full of
// streets that are not in the file.
func (n *network) compact() *network {
	used := make([]int32, len(n.names))
	for i := range used {
		used[i] = -1
	}
	var names []string
	for i, r := range n.roads {
		if used[r.name] == -1 {
			used[r.name] = int32(len(names))
			names = append(names, n.names[r.name])
		}
		n.roads[i].name = used[r.name]
	}
	n.names = names
	return n
}

// validate refuses to emit a network the simulator cannot demonstrate the product on. Each of these
// is a property the authored graph was checked for by hand, and the reason the check exists here is
// that a regenerated artefact is not read by anyone before it is committed.
func (n *network) validate() {
	area := contract.ServiceArea()

	inside, outside := 0, 0
	for _, at := range n.positions {
		if contract.Contains(area, at) {
			inside++
		} else {
			outside++
		}
	}
	log.Printf("%d intersections inside the service area, %d beyond it", inside, outside)

	if inside == 0 {
		log.Fatal("no intersection is inside the service area, so the fleet has nowhere to be placed")
	}
	if outside == 0 {
		log.Fatal("no intersection is beyond the service area, so a customer can never drive out of it")
	}

	for _, zone := range contract.Zones() {
		found := 0
		for _, at := range n.positions {
			if contract.Contains(zone.Boundary, at) {
				found++
			}
		}
		log.Printf("%s: %d intersections, minimum %d available", zone.Name, found, zone.Minimum)
		if found == 0 {
			log.Fatalf("zone %q contains no intersection, so its coverage would read as empty for the whole run", zone.ID)
		}
	}

	log.Printf("%d named streets, %d roads carrying no name", len(n.names), n.unnamed())
}

func (n *network) unnamed() int {
	count := 0
	for _, r := range n.roads {
		if n.names[r.name] == "" {
			count++
		}
	}
	return count
}

// write emits the artefact one entity per line: pretty-printing every coordinate onto its own line
// makes a 50,000-line diff out of a change to one road, and a single compact line makes the file
// unreadable and every regeneration a whole-file diff. A line per intersection and per road is the
// form in which a regeneration produces a diff a reviewer can actually read.
func (n *network) write(to io.Writer) {
	out := &strings.Builder{}

	fmt.Fprintf(out, "{\n  \"source\": {\n")
	fmt.Fprintf(out, "    \"data\": %q,\n", "© OpenStreetMap contributors")
	fmt.Fprintf(out, "    \"licence\": %q,\n", "ODbL 1.0 — https://opendatacommons.org/licenses/odbl/1-0/")
	fmt.Fprintf(out, "    \"endpoint\": %q,\n", endpoint)
	fmt.Fprintf(out, "    \"query\": %q,\n", query())
	fmt.Fprintf(out, "    \"built\": %q,\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(out, "    \"note\": %q\n", "Generated by cmd/roadgen; a derived database. Do not edit by hand.")
	fmt.Fprintf(out, "  },\n")

	fmt.Fprintf(out, "  \"names\": [\n")
	for i, street := range n.names {
		fmt.Fprintf(out, "    %s%s\n", quote(street), comma(i, len(n.names)))
	}
	fmt.Fprintf(out, "  ],\n")

	fmt.Fprintf(out, "  \"nodes\": [\n")
	for i, at := range n.positions {
		fmt.Fprintf(out, "    %s%s\n", coordinates(at), comma(i, len(n.positions)))
	}
	fmt.Fprintf(out, "  ],\n")

	fmt.Fprintf(out, "  \"roads\": [\n")
	for i, r := range n.roads {
		fmt.Fprintf(out, "    {\"a\":%d,\"b\":%d,\"name\":%d", r.a, r.b, r.name)
		if r.oneway {
			fmt.Fprintf(out, ",\"oneway\":true")
		}
		if len(r.via) > 0 {
			fmt.Fprintf(out, ",\"via\":[")
			for j, at := range r.via {
				fmt.Fprintf(out, "%s%s", coordinates(at), strings.TrimSpace(comma(j, len(r.via))))
			}
			fmt.Fprintf(out, "]")
		}
		fmt.Fprintf(out, "}%s\n", comma(i, len(n.roads)))
	}
	fmt.Fprintf(out, "  ]\n}\n")

	if _, err := io.WriteString(to, out.String()); err != nil {
		log.Fatalf("the network could not be written: %v", err)
	}
	log.Printf("wrote %.1f MB", float64(out.Len())/(1<<20))
}

func quote(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		log.Fatalf("a street name will not serialise: %v", err)
	}
	return string(encoded)
}

func coordinates(at contract.Point) string {
	return "[" + number(at.Lng()) + "," + number(at.Lat()) + "]"
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func comma(i, length int) string {
	if i+1 == length {
		return ""
	}
	return ","
}
