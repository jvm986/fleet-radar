// Package fleet holds the fleet's state and the projection that writes to it. Events are
// the only writer, and this is the package where that is a property of the types rather
// than a convention (ADR-0004 §4.3).
//
// Nothing here derives anything. Attention conditions, zone membership, coverage and
// summary counts are computed from a snapshot every tick and never stored, so there is no
// cache to invalidate and no scheduled work (ADR-0004 §4.2, §4.8, §4.9).
package fleet

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"fleetradar/contract"
)

// Outcome is what became of one delivery. Duplicate and Superseded are the ordinary
// consequences of at-least-once, unordered delivery rather than faults, and they are
// reported so that correct handling is observable in the log instead of merely asserted
// (PRODUCT-SPEC F9).
type Outcome string

const (
	Applied Outcome = "applied"
	// Duplicate is a redelivery of the value already held: the same sequence number for the
	// same signal.
	Duplicate Outcome = "duplicate"
	// Superseded is an older observation arriving late, after a newer one has landed.
	Superseded Outcome = "superseded"
	// UnknownVehicle is telemetry for a vehicle that has not been registered. Because
	// delivery is unordered this happens legitimately at startup, and it is harmless: at
	// 1 Hz the next report is a second away (ADR-0003 §3.11).
	UnknownVehicle Outcome = "unknown_vehicle"
	// Uninterpretable is a delivery that could not be read as an event at all.
	Uninterpretable Outcome = "uninterpretable"
)

// Vehicle is one vehicle's state as the store holds it: the latest absolute value for each
// signal, and nothing else.
type Vehicle struct {
	ID    string
	Label string

	Position contract.Point
	Heading  float64
	Status   contract.VehicleStatus
	Battery  float64
	// Route is the zero value when the vehicle has none. Its geometry is replaced wholesale
	// rather than mutated, so a published snapshot may share it.
	Route contract.Route

	// Reporting is true once position, battery and status have each been observed at least
	// once — the point at which the vehicle can be drawn. Until then it is a vehicle we know
	// exists and have never heard from, which is a different thing from one we have stopped
	// hearing from (PRODUCT-SPEC F6, ADR-0004 §4.8).
	Reporting bool

	// LastObserved is the newest observation timestamp among the vehicle's own telemetry.
	// Registration is excluded because a replayed roster entry is not the vehicle speaking,
	// and route events are excluded because they come from the assignment system: either
	// would let a silent vehicle look as though it had just reported.
	LastObserved time.Time
}

// Reader is the read-only view the serving layer holds. "Events are the only writer of
// state" is otherwise a convention that lasts until somebody adds a handler which mutates
// state directly; here the write methods are simply not reachable from the read path
// (ADR-0004 §4.3, ADR-0009 §9.8).
type Reader interface {
	// Snapshot returns every registered vehicle, ordered by identity. Neither the slice nor
	// anything reachable from it is written to afterwards, so a reader consumes it without
	// locking and every reader gets an identical view (ADR-0004 §4.4).
	Snapshot() []Vehicle
}

// MemoryStore holds signal state, sequence numbers and observation timestamps, and it is where
// synchronisation lives: writes serialise on the mutex, and readers take an immutable snapshot rather
// than contending for it (ADR-0004 §4.4).
//
// The brief allows in-memory storage, and this is that permission taken deliberately rather than
// assumed: in an event-sourced design the stream is the recovery mechanism, so persistence would be a
// second answer to a question already answered (PRODUCT-SPEC §7.4). A persistent store would replace
// this type; nothing above it would change, because the only thing above it is the projection and the
// read-only view.
//
// ADR-0004 wrapped this in a FleetStore port, on the grounds that the port owned concurrency and that
// tests would supply a second implementation. Once written, neither held: concurrency is this struct's
// property whether or not an interface names it, and no second implementation exists. So the interface
// was removed rather than defended, which is what ADR-0010 §10.5 asked for.
type MemoryStore struct {
	mu       sync.Mutex
	vehicles map[string]*vehicleState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{vehicles: map[string]*vehicleState{}}
}

// register is one signal's last-write-wins slot. Strictly-newer-wins over an absolute value
// is the whole of the ordering and duplicate-suppression model: applying the same event
// twice leaves the same state, so at-least-once delivery is satisfied structurally, with no
// seen-set to grow, no age cutoff and no eviction policy to tune (ADR-0003 §3.3, §3.4).
type register[T any] struct {
	value      T
	sequence   uint64
	observedAt time.Time
	present    bool
}

func (r *register[T]) accept(sequence uint64, observedAt time.Time, value T) Outcome {
	if r.present {
		switch {
		case sequence == r.sequence:
			return Duplicate
		case sequence < r.sequence:
			return Superseded
		}
	}
	*r = register[T]{value: value, sequence: sequence, observedAt: observedAt, present: true}
	return Applied
}

type positionValue struct {
	point   contract.Point
	heading float64
}

// vehicleState is five independent registers. Each signal is compared only against itself,
// which is why the sequence number is per vehicle per signal: under one counter per vehicle,
// a battery event at sequence 50 would make an unrelated position event at 49 look
// superseded (ADR-0003 §3.4).
type vehicleState struct {
	id           string
	registration register[string]
	position     register[positionValue]
	battery      register[float64]
	status       register[contract.VehicleStatus]
	// route holds the assigned route, or the zero value once cleared. RouteAssigned and
	// RouteCleared share this one register, so a clear that arrives out of order is compared
	// against the assignment it would undo (ADR-0003 §3.8).
	route register[contract.Route]
}

func (v *vehicleState) snapshot() Vehicle {
	snapshot := Vehicle{
		ID:        v.id,
		Label:     v.registration.value,
		Position:  v.position.value.point,
		Heading:   v.position.value.heading,
		Status:    v.status.value,
		Battery:   v.battery.value,
		Route:     v.route.value,
		Reporting: v.position.present && v.battery.present && v.status.present,
	}
	for _, telemetry := range []struct {
		present    bool
		observedAt time.Time
	}{
		{v.position.present, v.position.observedAt},
		{v.battery.present, v.battery.observedAt},
		{v.status.present, v.status.observedAt},
	} {
		if telemetry.present && telemetry.observedAt.After(snapshot.LastObserved) {
			snapshot.LastObserved = telemetry.observedAt
		}
	}
	return snapshot
}

// Register is the only way a vehicle comes into existence. The backend learns the fleet from
// registrations replayed at startup, which is what makes a vehicle that was already silent
// visible as registered rather than absent (ADR-0003 §3.9).
func (s *MemoryStore) Register(e contract.Envelope, p contract.RegisteredPayload) Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	vehicle, known := s.vehicles[e.VehicleID]
	if !known {
		vehicle = &vehicleState{id: e.VehicleID}
		s.vehicles[e.VehicleID] = vehicle
	}
	return vehicle.registration.accept(e.Sequence, e.ObservedAt, p.Label)
}

func (s *MemoryStore) SetPosition(e contract.Envelope, p contract.PositionPayload) Outcome {
	return s.write(e.VehicleID, func(v *vehicleState) Outcome {
		return v.position.accept(e.Sequence, e.ObservedAt, positionValue{point: p.Position, heading: p.Heading})
	})
}

func (s *MemoryStore) SetBattery(e contract.Envelope, p contract.BatteryPayload) Outcome {
	return s.write(e.VehicleID, func(v *vehicleState) Outcome {
		return v.battery.accept(e.Sequence, e.ObservedAt, p.Percent)
	})
}

func (s *MemoryStore) SetStatus(e contract.Envelope, p contract.StatusPayload) Outcome {
	return s.write(e.VehicleID, func(v *vehicleState) Outcome {
		return v.status.accept(e.Sequence, e.ObservedAt, p.Status)
	})
}

func (s *MemoryStore) SetRoute(e contract.Envelope, route contract.Route) Outcome {
	return s.write(e.VehicleID, func(v *vehicleState) Outcome {
		return v.route.accept(e.Sequence, e.ObservedAt, route)
	})
}

func (s *MemoryStore) ClearRoute(e contract.Envelope) Outcome {
	return s.write(e.VehicleID, func(v *vehicleState) Outcome {
		return v.route.accept(e.Sequence, e.ObservedAt, contract.Route{})
	})
}

// write is where the unknown-vehicle rule lives, once, so that no signal can accidentally
// bring a vehicle into existence from a stray identifier.
func (s *MemoryStore) write(vehicleID string, apply func(*vehicleState) Outcome) Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	vehicle, known := s.vehicles[vehicleID]
	if !known {
		return UnknownVehicle
	}
	return apply(vehicle)
}

func (s *MemoryStore) Snapshot() []Vehicle {
	s.mu.Lock()
	snapshot := make([]Vehicle, 0, len(s.vehicles))
	for _, vehicle := range s.vehicles {
		snapshot = append(snapshot, vehicle.snapshot())
	}
	s.mu.Unlock()

	slices.SortFunc(snapshot, func(a, b Vehicle) int { return cmp.Compare(a.ID, b.ID) })
	return snapshot
}
