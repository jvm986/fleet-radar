package sim

import (
	"math"
	"math/rand/v2"
	"time"

	"fleetradar/contract"
)

// The state machine is the teledriving lifecycle itself, which is the domain model confirmed in
// PRODUCT-SPEC §6.1.1 rather than a convenience:
//
//	FREE ──assigned──▶ EN_ROUTE (to the customer) ──arrives──▶ WITH_CUSTOMER
//	  ▲                                                              │
//	  └──arrives── EN_ROUTE (to parking) ◀──── customer done ─────────┘
//
// Route clearing falls out of it rather than needing a rule of its own: a customer-driven vehicle
// has no plan for the system to hold, so the route ends when the customer takes over
// (ADR-0007 §7.5).
//
// One thing differs from the ADR's diagram, and it is what makes "~10 simultaneously EN_ROUTE" hold
// as an invariant. There, a finished customer trip becomes the return leg directly — but that leg is
// also EN_ROUTE, and it is entered without the scheduler's say-so, so the count drifts to ten
// dispatched plus everyone driving back. Instead the customer's drop-off leaves the vehicle parked
// and available where it stands, and the return leg is a journey the scheduler assigns like any
// other. Both meanings of EN_ROUTE the spec gives — towards a customer, or away again — still occur,
// and every transition is now the scheduler's (PRODUCT-SPEC §6.1.1).
type leg int

const (
	legParked leg = iota
	legToCustomer
	legWithCustomer
	legToParking
)

func (l leg) status() contract.VehicleStatus {
	switch l {
	case legToCustomer, legToParking:
		return contract.StatusEnRoute
	case legWithCustomer:
		return contract.StatusWithCustomer
	}
	return contract.StatusFree
}

// event is one signal a vehicle has produced. It carries no sequence number: the simulator allocates one
// from the vehicle's counter for the event type's signal, so which register an event belongs to is stated
// once — by contract.EventType.Signal — rather than at every place an event is produced (ADR-0003 §3.4).
type event struct {
	eventType contract.EventType
	payload   any
}

type vehicle struct {
	id    string
	label string

	leg     leg
	node    NodeID
	at      contract.Point
	heading float64
	// path is the intersections still to be reached, excluding the one just passed.
	path    []NodeID
	routeID string

	battery  float64
	charging bool

	// slot is which tick of the reporting interval this vehicle speaks on. Staggering the fleet
	// across the interval is what makes the event rate smooth rather than a burst per second
	// (ADR-0007 §7.14).
	slot        int
	reports     uint64
	silentUntil time.Time
	// mute never reports at all. One vehicle is silent from startup so that "registered but never
	// reported" is observable — a specified state that cannot be seen is worse than one that does
	// not exist (ADR-0007 §7.7).
	mute bool
	// reported is the status last sent, so that status is emitted on change (ADR-0003 §3.13).
	reported contract.VehicleStatus

	sequences map[contract.Signal]uint64
}

// next is the producer's obligation: a counter per vehicle per signal, monotonic, which is the only
// relationship the backend may rely on (ADR-0003 §3.16).
func (v *vehicle) next(signal contract.Signal) uint64 {
	v.sequences[signal]++
	return v.sequences[signal]
}

// advance moves the vehicle and spends its energy. Route events come out of it, because arriving
// somewhere is what ends and begins a journey; telemetry does not, because that is reported on
// the vehicle's own cadence.
func (v *vehicle) advance(dt time.Duration, graph *Graph, random *rand.Rand) []event {
	if len(v.path) == 0 {
		v.park(dt)
		return nil
	}

	remaining := Speed * dt.Seconds()
	travelled := 0.0
	for remaining > 0 && len(v.path) > 0 {
		target := graph.Position(v.path[0])
		step := metresBetween(v.at, target)
		if step > 0 {
			v.heading = bearingTo(v.at, target)
		}
		if step > remaining {
			v.at, _ = towards(v.at, target, remaining)
			travelled += remaining
			break
		}

		v.at, v.node = target, v.path[0]
		v.path = v.path[1:]
		travelled += step
		remaining -= step
	}

	v.battery = max(v.battery-travelled/1000*DrainPerKm, 0)
	if len(v.path) == 0 {
		return v.arrive(graph, random)
	}
	return nil
}

// park drains a stationary vehicle slowly, and charges one that has run down.
//
// Energy recovery exists because without it the fleet dies: charging locations are out of scope, so
// drain alone means every vehicle eventually reaches zero. A vehicle below the floor stays parked
// and its energy rises, and it stays FREE throughout because no charging status exists and the spec
// excludes inventing one. The operator sees a battery increase, which is honest — a parked vehicle
// being charged is exactly what would be happening (ADR-0007 §7.6).
func (v *vehicle) park(dt time.Duration) {
	if v.battery <= ChargeFloor {
		v.charging = true
	}
	if v.charging {
		v.battery = min(v.battery+ChargePerMinute*dt.Minutes(), ChargeTarget)
		if v.battery >= ChargeTarget {
			v.charging = false
		}
		return
	}
	v.battery = max(v.battery-IdleDrainPerMinute*dt.Minutes(), 0)
}

func (v *vehicle) arrive(graph *Graph, random *rand.Rand) []event {
	switch v.leg {
	case legToCustomer:
		v.leg = legWithCustomer
		v.path = v.customerTrip(graph, random)
		return []event{v.clearRoute()}

	case legWithCustomer:
		// The customer has arrived and got out, so the vehicle is parked and available where it
		// stands. There is no route to clear: it ended when the customer took over.
		v.leg = legParked

	case legToParking:
		v.leg = legParked
		return []event{v.clearRoute()}
	}
	return nil
}

// dispatchTo sends a parked vehicle on a journey, either to a customer or to reposition itself.
func (v *vehicle) dispatchTo(purpose leg, graph *Graph, random *rand.Rand) []event {
	v.path = v.plan(v.node, graph, random)
	if len(v.path) == 0 {
		return nil
	}
	v.leg = purpose
	return v.assignRoute(graph, random)
}

// replan revises a journey in progress, keeping the segment already being driven. A journey can be
// revised and the operator needs the current intent rather than the original one, and without this
// the route-update path would never run outside a test (PRODUCT-SPEC §2.2, F2).
func (v *vehicle) replan(graph *Graph, random *rand.Rand) []event {
	if v.leg.status() != contract.StatusEnRoute || len(v.path) == 0 {
		return nil
	}
	if random.Float64() >= ReassignChancePerReport {
		return nil
	}

	ahead := v.path[0]
	rest := v.plan(ahead, graph, random)
	if len(rest) == 0 {
		return nil
	}
	v.path = append([]NodeID{ahead}, rest...)
	return v.assignRoute(graph, random)
}

// report is the vehicle's own telemetry, and the only thing silence suppresses. Route events keep
// flowing while a vehicle is quiet, because they come from the assignment system rather than from
// the vehicle — which is also why staleness does not count them.
func (v *vehicle) report(now time.Time, random *rand.Rand) []event {
	if v.mute || now.Before(v.silentUntil) {
		return nil
	}
	// Silence has to be modelled, because nothing about the simulated world causes it, and without
	// it the state the freshness feature exists for would never occur (ADR-0007 §7.7).
	if random.Float64() < SilenceChance {
		v.silentUntil = now.Add(SilenceMin + time.Duration(random.Int64N(int64(SilenceMax-SilenceMin))))
		return nil
	}

	events := []event{{
		eventType: contract.EventVehiclePosition,
		payload:   contract.PositionPayload{Position: v.at, Heading: v.heading},
	}}
	if v.reports%batteryEveryNReports == 0 {
		events = append(events, event{
			eventType: contract.EventVehicleBattery,
			payload:   contract.BatteryPayload{Percent: v.battery},
		})
	}
	if status := v.leg.status(); status != v.reported {
		v.reported = status
		events = append(events, event{
			eventType: contract.EventVehicleStatus,
			payload:   contract.StatusPayload{Status: status},
		})
	}

	v.reports++
	return events
}

func (v *vehicle) assignRoute(graph *Graph, random *rand.Rand) []event {
	if len(v.path) == 0 {
		return nil
	}

	geometry := make([]contract.Point, 0, len(v.path)+1)
	geometry = append(geometry, v.at)
	for _, id := range v.path {
		geometry = append(geometry, graph.Position(id))
	}

	v.routeID = uuid(random)
	return []event{{
		eventType: contract.EventRouteAssigned,
		payload: contract.Route{
			RouteID:     v.routeID,
			Geometry:    geometry,
			Destination: geometry[len(geometry)-1],
		},
	}}
}

func (v *vehicle) clearRoute() event {
	cleared := v.routeID
	v.routeID = ""
	return event{
		eventType: contract.EventRouteCleared,
		payload:   contract.RouteClearedPayload{RouteID: cleared},
	}
}

// customerTrip is where the customer decides to go, which may be outside the service area
// entirely. A customer driving may go anywhere, so the map must not pretend otherwise — and it has
// to actually happen, or a specified behaviour is undemonstrable (PRODUCT-SPEC §2.5,
// ADR-0007 §7.12).
func (v *vehicle) customerTrip(graph *Graph, random *rand.Rand) []NodeID {
	if random.Float64() < LeavesServiceAreaChance {
		// A reachable exit first: the network leaves the service area in more than one direction, and
		// sending a vehicle in the west across the whole map to the eastern edge would make leaving the
		// area take half an hour rather than a few minutes.
		if path := graph.Path(v.node, graph.Outside(), 0, TripMaxMetres, random); len(path) > 0 {
			return path
		}
		if path := graph.Path(v.node, graph.Outside(), 0, math.Inf(1), random); len(path) > 0 {
			return path
		}
	}
	return v.plan(v.node, graph, random)
}

// plan chooses somewhere within a journey's reach, inside the service area, and widens the search rather
// than giving up: a vehicle with nowhere to go would sit still for the rest of the run.
func (v *vehicle) plan(from NodeID, graph *Graph, random *rand.Rand) []NodeID {
	if path := graph.Path(from, graph.Inside(), TripMinMetres, TripMaxMetres, random); len(path) > 0 {
		return path
	}
	return graph.Path(from, graph.Inside(), 0, math.Inf(1), random)
}
