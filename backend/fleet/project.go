package fleet

import (
	"encoding/json"
	"log/slog"
	"time"

	"fleetradar/contract"
)

// payloadExcerpt bounds how much of an unreadable delivery reaches the log. Enough to
// recognise what arrived, not enough for one bad producer to flood the log (ADR-0003 §3.11).
const payloadExcerpt = 256

// Record is one delivery from the stream. The ingest boundary is shaped as a consumer over
// (topic, key, payload), so a real Kafka client substitutes for the simulated source without
// the projection changing — and because serialised bytes cross that boundary, validation and
// the discard path below are genuinely executed by the running system rather than only by
// tests (ADR-0003 §3.16, ADR-0007 §7.2).
type Record struct {
	Topic contract.Topic
	// Key is the partition key, which is the vehicle identity. That is what makes
	// per-vehicle sequencing the right granularity, and what would let ingest partition
	// across consumers with no cross-partition coordination (ADR-0003 §3.16).
	Key     string
	Payload []byte
}

// Projector is the only writer of fleet state.
type Projector struct {
	store FleetStore
	log   *slog.Logger
	// now exists so the implausible-timestamp check is testable. The observation clock is
	// authoritative regardless of what it says; receive time is only ever a diagnostic
	// (ADR-0003 §3.5, ADR-0009 §9.12).
	now func() time.Time
}

func NewProjector(store FleetStore, log *slog.Logger, now func() time.Time) *Projector {
	return &Projector{store: store, log: log, now: now}
}

// projected is the result of reading one delivery: what became of it, why if it was
// uninterpretable, and the route it named if it named one — carried so that a discarded route
// event says in the log which route it was about (ADR-0003 §3.8).
type projected struct {
	outcome Outcome
	reason  string
	routeID string
}

// Apply projects one delivery and reports what became of it. One bad event must never stop
// the fleet, so every failure is a discard with a logged reason rather than an error
// returned upwards (ADR-0003 §3.11).
func (p *Projector) Apply(r Record) Outcome {
	var e contract.Envelope
	if err := json.Unmarshal(r.Payload, &e); err != nil {
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "delivery is not valid JSON"})
	}

	expectedTopic, known := e.Type.Topic()
	switch {
	case !known:
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "unknown event type"})
	case expectedTopic != r.Topic:
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "event type on the wrong topic"})
	case e.VehicleID == "":
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "event names no vehicle"})
	case r.Key != e.VehicleID:
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "partition key is not the vehicle it names"})
	case e.ObservedAt.IsZero():
		// Without an observation timestamp there is no answer to how current this is, and
		// staleness is the guarantee the rest of the view rests on.
		return p.discard(r, e, projected{outcome: Uninterpretable, reason: "event has no observation timestamp"})
	}

	if e.ObservedAt.After(p.now()) {
		// Applied anyway: the producer's clock is authoritative. Logged because a producer
		// running ahead makes staleness unreachable, which is otherwise silent.
		p.log.Warn("observation timestamp is in the future",
			slog.String("vehicleId", e.VehicleID),
			slog.String("type", string(e.Type)),
			slog.Time("observedAt", e.ObservedAt),
		)
	}

	result := p.project(e)
	if result.outcome != Applied {
		return p.discard(r, e, result)
	}
	return Applied
}

func (p *Projector) project(e contract.Envelope) projected {
	switch e.Type {
	case contract.EventVehicleRegistered:
		var payload contract.RegisteredPayload
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return projected{outcome: Uninterpretable, reason: "registration payload is unreadable"}
		}
		if payload.Label == "" {
			// The label is what the operator says aloud to hand a vehicle over, so a
			// registration without one is not usable (PRODUCT-SPEC §7.3).
			return projected{outcome: Uninterpretable, reason: "registration carries no label"}
		}
		return projected{outcome: p.store.Register(e, payload)}

	case contract.EventVehiclePosition:
		var payload contract.PositionPayload
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return projected{outcome: Uninterpretable, reason: "position payload is unreadable"}
		}
		if !plausiblePoint(payload.Position) {
			return projected{outcome: Uninterpretable, reason: "position is not a coordinate"}
		}
		if payload.Heading < 0 || payload.Heading >= 360 {
			return projected{outcome: Uninterpretable, reason: "heading is not a bearing"}
		}
		return projected{outcome: p.store.SetPosition(e, payload)}

	case contract.EventVehicleBattery:
		var payload contract.BatteryPayload
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return projected{outcome: Uninterpretable, reason: "battery payload is unreadable"}
		}
		if payload.Percent < 0 || payload.Percent > 100 {
			return projected{outcome: Uninterpretable, reason: "battery is not a proportion of capacity"}
		}
		return projected{outcome: p.store.SetBattery(e, payload)}

	case contract.EventVehicleStatus:
		var payload contract.StatusPayload
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return projected{outcome: Uninterpretable, reason: "status payload is unreadable"}
		}
		switch payload.Status {
		case contract.StatusFree, contract.StatusEnRoute, contract.StatusWithCustomer:
		default:
			return projected{outcome: Uninterpretable, reason: "status is not one of the three"}
		}
		return projected{outcome: p.store.SetStatus(e, payload)}

	case contract.EventRouteAssigned:
		var route contract.Route
		if err := json.Unmarshal(e.Payload, &route); err != nil {
			return projected{outcome: Uninterpretable, reason: "route payload is unreadable"}
		}
		if route.RouteID == "" {
			return projected{outcome: Uninterpretable, reason: "route has no id"}
		}
		if len(route.Geometry) < 2 {
			return projected{outcome: Uninterpretable, reason: "route is not a path", routeID: route.RouteID}
		}
		return projected{outcome: p.store.SetRoute(e, route), routeID: route.RouteID}

	case contract.EventRouteCleared:
		var payload contract.RouteClearedPayload
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return projected{outcome: Uninterpretable, reason: "route payload is unreadable"}
		}
		if payload.RouteID == "" {
			return projected{outcome: Uninterpretable, reason: "route has no id"}
		}
		return projected{outcome: p.store.ClearRoute(e), routeID: payload.RouteID}
	}

	return projected{outcome: Uninterpretable, reason: "no projection for this event type"}
}

// discard logs why a delivery changed nothing. Duplicates and superseded observations are the
// designed behaviour of at-least-once, unordered delivery, so they are logged at info: they
// are the only place that handling is observable, and logging them as problems would make a
// healthy system look broken (ADR-0007 §7.7, ADR-0009 §9.7).
func (p *Projector) discard(r Record, e contract.Envelope, result projected) Outcome {
	attrs := []any{
		slog.String("reason", string(result.outcome)),
		slog.String("topic", string(r.Topic)),
		slog.String("type", string(e.Type)),
		slog.String("vehicleId", e.VehicleID),
		slog.String("eventId", e.EventID),
		slog.Uint64("sequence", e.Sequence),
	}
	if result.routeID != "" {
		attrs = append(attrs, slog.String("routeId", result.routeID))
	}
	if result.reason != "" {
		attrs = append(attrs, slog.String("detail", result.reason))
	}
	if result.outcome == Uninterpretable {
		attrs = append(attrs, slog.String("payload", excerpt(r.Payload)))
	}

	switch result.outcome {
	case Duplicate, Superseded:
		p.log.Info("discarded", attrs...)
	default:
		p.log.Warn("discarded", attrs...)
	}
	return result.outcome
}

func plausiblePoint(p contract.Point) bool {
	return p.Lng() >= -180 && p.Lng() <= 180 && p.Lat() >= -90 && p.Lat() <= 90
}

func excerpt(payload []byte) string {
	if len(payload) > payloadExcerpt {
		return string(payload[:payloadExcerpt]) + "…"
	}
	return string(payload)
}
