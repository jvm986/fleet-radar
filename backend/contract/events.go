package contract

import (
	"encoding/json"
	"time"
)

// Topic is a logical Kafka topic. Kafka's shape is modelled where it constrains the
// design and nowhere else (ADR-0003 §3.16): topics, vehicle UUID as the partition key,
// at-least-once delivery, and compaction semantics behind registration replay. Offsets,
// consumer groups, rebalancing and dead-lettering are named in the ADR, not built.
type Topic string

const (
	TopicLifecycle Topic = "vehicle.lifecycle"
	TopicTelemetry Topic = "vehicle.telemetry"
	TopicRoutes    Topic = "vehicle.routes"
)

// EventType names one of the six events the backend consumes (ADR-0003 §3.1).
type EventType string

const (
	EventVehicleRegistered EventType = "VehicleRegistered"
	EventVehiclePosition   EventType = "VehiclePosition"
	EventVehicleBattery    EventType = "VehicleBattery"
	EventVehicleStatus     EventType = "VehicleStatus"
	EventRouteAssigned     EventType = "RouteAssigned"
	EventRouteCleared      EventType = "RouteCleared"
)

// Signal names the last-write-wins register an event writes to. Sequence numbers are per
// vehicle per signal, so each signal is only ever compared against itself: under a single
// per-vehicle counter, a battery event at sequence 50 would make an unrelated position
// event at 49 look superseded (ADR-0003 §3.4).
type Signal string

const (
	SignalRegistration Signal = "registration"
	SignalPosition     Signal = "position"
	SignalBattery      Signal = "battery"
	SignalStatus       Signal = "status"
	SignalRoute        Signal = "route"
)

// Signal reports which register the event writes to, and whether the type is one the
// backend understands at all — an unrecognised type is uninterpretable and is discarded
// with a reason (ADR-0003 §3.11).
//
// RouteAssigned and RouteCleared share one register deliberately: they are two states of
// the same signal, so a clear that arrives out of order is compared against the
// assignment it would undo rather than against nothing.
func (t EventType) Signal() (Signal, bool) {
	switch t {
	case EventVehicleRegistered:
		return SignalRegistration, true
	case EventVehiclePosition:
		return SignalPosition, true
	case EventVehicleBattery:
		return SignalBattery, true
	case EventVehicleStatus:
		return SignalStatus, true
	case EventRouteAssigned, EventRouteCleared:
		return SignalRoute, true
	}
	return "", false
}

// Topic reports which topic the event belongs on, and whether the type is one the backend
// understands at all. Ingest rejects a known type arriving on the wrong topic, which is the
// only reason the delivery's topic is more than decoration.
func (t EventType) Topic() (Topic, bool) {
	switch t {
	case EventVehicleRegistered:
		return TopicLifecycle, true
	case EventVehiclePosition, EventVehicleBattery, EventVehicleStatus:
		return TopicTelemetry, true
	case EventRouteAssigned, EventRouteCleared:
		return TopicRoutes, true
	}
	return "", false
}

// Envelope is every event on the wire. The fields below are common to all six types; the
// signal itself travels as raw JSON and is decoded against Type.
type Envelope struct {
	// EventID identifies this delivery. It plays no part in ordering or in duplicate
	// suppression — it exists so that a discard can be logged meaningfully and so one
	// event can be traced end to end (ADR-0003 §3.2).
	EventID string `json:"eventId"`

	Type EventType `json:"type"`

	// VehicleID is the stable machine identity: what events are addressed to, what the
	// stream is partitioned by, and what never changes. The human-readable label the
	// operator says aloud travels on registration instead (PRODUCT-SPEC §7.3).
	VehicleID string `json:"vehicleId"`

	// Sequence is monotonic per vehicle per signal from a given producer. It is the only
	// relationship the backend may rely on, and strictly-newer-wins over it is what makes
	// unordered, at-least-once delivery safe (ADR-0003 §3.4, §3.16).
	Sequence uint64 `json:"sequence"`

	// ObservedAt is the producer's observation clock, and it is authoritative — including
	// for staleness, because the operator's question is how current the information about
	// the vehicle is, not when we happened to receive it. Backend receive time is stamped
	// at ingest and used only to diagnose clock skew (ADR-0003 §3.5, §3.7).
	ObservedAt time.Time `json:"observedAt"`

	Payload json.RawMessage `json:"payload"`
}

// Every payload below carries absolute values only. This is load-bearing rather than
// stylistic: applying an absolute value under strictly-newer-wins is idempotent, which is
// what satisfies at-least-once delivery with no dedup cache, no memory growth and no
// eviction policy. A single field expressed as an increment anywhere would be corrupted
// by redelivery and would force all of that back into existence (ADR-0003 §3.3).

// RegisteredPayload carries the vehicle's human-readable label — the short identifier the
// operator reads off the screen and says aloud on a radio. Because it lives on
// registration rather than on every event, a label can be corrected without touching
// identity or invalidating anything already delivered (PRODUCT-SPEC §7.3).
type RegisteredPayload struct {
	Label string `json:"label"`
}

// PositionPayload carries position and heading together because they come from a single
// sensor read; splitting them would open a window in which heading disagrees with
// position, for no benefit (ADR-0003 §3.1).
type PositionPayload struct {
	Position Point `json:"position"`
	// Heading is degrees clockwise from north. A stationary vehicle's heading tells the
	// operator which way it will leave, so it is meaningful for every vehicle rather than
	// only for moving ones (PRODUCT-SPEC §2.1).
	Heading float64 `json:"heading"`
}

// BatteryPayload carries energy as a proportion of capacity, 0 to 100.
type BatteryPayload struct {
	Percent float64 `json:"percent"`
}

type StatusPayload struct {
	Status VehicleStatus `json:"status"`
}

// A RouteAssigned event carries a Route, which is declared alongside the rest of the shared
// vocabulary because the same value travels on to the client unchanged.

// RouteClearedPayload is a distinct event rather than an assignment with empty geometry,
// so that "this vehicle has no route" and "this update does not mention a route" stay
// distinguishable.
//
// RouteID is what makes an out-of-order clear safe: a clear applies only when it names
// the route currently held, so it cannot wipe a route assigned after it (ADR-0003 §3.8).
type RouteClearedPayload struct {
	RouteID string `json:"routeId"`
}
