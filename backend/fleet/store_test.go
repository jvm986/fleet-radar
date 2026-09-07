package fleet

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"fleetradar/contract"
)

// Permutation invariance is the test because it is the claim: ADR-0003 asserts that applying
// a set of events in any order, with arbitrary duplication, yields identical state. If this
// ever fails it means a value has stopped being absolute somewhere, which is the single thing
// identified as breaking the model (ADR-0009 §9.3).
//
// Registrations are applied first rather than shuffled in with the rest, because the source
// guarantees that: replay completes before telemetry begins. Telemetry arriving before its
// registration is legitimately discarded, and has its own test below.
func TestProjectionIsPermutationInvariant(t *testing.T) {
	registrations := []Record{
		registration(t, vehicleOne, "LV-001"),
		registration(t, vehicleTwo, "LV-002"),
	}
	telemetry := []Record{
		position(t, vehicleOne, 1, at(1), -115.172, 36.114, 90),
		position(t, vehicleOne, 2, at(2), -115.171, 36.115, 92),
		position(t, vehicleOne, 3, at(3), -115.170, 36.116, 95),
		battery(t, vehicleOne, 1, at(1), 88),
		battery(t, vehicleOne, 2, at(11), 86.5),
		status(t, vehicleOne, 1, at(2), contract.StatusEnRoute),
		status(t, vehicleOne, 2, at(9), contract.StatusWithCustomer),
		routeAssigned(t, vehicleOne, 1, at(2), "route-a"),
		routeCleared(t, vehicleOne, 2, at(9), "route-a"),
		position(t, vehicleTwo, 1, at(1), -115.148, 36.163, 270),
		position(t, vehicleTwo, 2, at(2), -115.149, 36.163, 268),
		battery(t, vehicleTwo, 1, at(1), 17),
		status(t, vehicleTwo, 1, at(1), contract.StatusFree),
		routeAssigned(t, vehicleTwo, 1, at(3), "route-b"),
	}

	want := projectFresh(t, registrations, telemetry)

	random := rand.New(rand.NewPCG(0x5EED, 0xF1EE7))
	for run := range 200 {
		shuffled := slices.Clone(telemetry)
		random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		delivered := make([]Record, 0, len(shuffled)*2)
		for _, r := range shuffled {
			delivered = append(delivered, r)
			if random.IntN(3) == 0 {
				delivered = append(delivered, r)
			}
		}

		if got := projectFresh(t, registrations, delivered); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d ended in different state\n got: %+v\nwant: %+v", run, got, want)
		}
	}
}

func TestOrdering(t *testing.T) {
	t.Run("a later observation is not replaced by an earlier one", func(t *testing.T) {
		store, projector := freshStore(t)
		applyAll(t, projector,
			position(t, vehicleOne, 5, at(5), -115.170, 36.116, 90),
			position(t, vehicleOne, 4, at(4), -115.180, 36.120, 180),
		)
		if got := store.Snapshot()[0].Position; got != (contract.Point{-115.170, 36.116}) {
			t.Errorf("Position = %v, want the observation at sequence 5", got)
		}
	})

	t.Run("a redelivery is discarded because it is not newer", func(t *testing.T) {
		store, projector := freshStore(t)
		first := position(t, vehicleOne, 5, at(5), -115.170, 36.116, 90)

		if got := projector.Apply(first); got != Applied {
			t.Fatalf("first delivery = %q, want %q", got, Applied)
		}
		if got := projector.Apply(first); got != Duplicate {
			t.Errorf("second delivery = %q, want %q", got, Duplicate)
		}
		if got := store.Snapshot()[0].Position; got != (contract.Point{-115.170, 36.116}) {
			t.Errorf("Position = %v, want it unchanged by the redelivery", got)
		}
	})

	t.Run("signals are sequenced independently of each other", func(t *testing.T) {
		store, projector := freshStore(t)
		applyAll(t, projector,
			battery(t, vehicleOne, 50, at(50), 41),
			position(t, vehicleOne, 49, at(49), -115.170, 36.116, 90),
		)

		vehicle := store.Snapshot()[0]
		if vehicle.Battery != 41 {
			t.Errorf("Battery = %v, want 41", vehicle.Battery)
		}
		if vehicle.Position != (contract.Point{-115.170, 36.116}) {
			t.Error("a position at sequence 49 was superseded by a battery event at 50")
		}
	})

	t.Run("a route cleared out of order is not undone by its own assignment", func(t *testing.T) {
		store, projector := freshStore(t)
		applyAll(t, projector,
			routeCleared(t, vehicleOne, 2, at(9), "route-a"),
			routeAssigned(t, vehicleOne, 1, at(2), "route-a"),
		)
		if got := store.Snapshot()[0].Route.RouteID; got != "" {
			t.Errorf("RouteID = %q, want no route: the clear is the newer observation", got)
		}
	})

	t.Run("clearing a route removes the geometry with it", func(t *testing.T) {
		store, projector := freshStore(t)
		applyAll(t, projector,
			routeAssigned(t, vehicleOne, 1, at(2), "route-a"),
			routeCleared(t, vehicleOne, 2, at(9), "route-a"),
		)
		if got := store.Snapshot()[0].Route; got.RouteID != "" || got.Geometry != nil {
			t.Errorf("Route = %+v, want the zero value", got)
		}
	})
}

// A stray or malformed identifier must not become a vehicle on the operator's map, so the
// backend learns the fleet from registrations and from nothing else (ADR-0003 §3.9, §3.11).
func TestTelemetryForAnUnregisteredVehicleIsDiscarded(t *testing.T) {
	store := NewMemoryStore()
	projector, _ := newProjector(t, store)

	if got := projector.Apply(position(t, vehicleOne, 1, at(1), -115.17, 36.11, 90)); got != UnknownVehicle {
		t.Errorf("Apply() = %q, want %q", got, UnknownVehicle)
	}
	if snapshot := store.Snapshot(); len(snapshot) != 0 {
		t.Errorf("Snapshot() = %+v, want no vehicles", snapshot)
	}
}

// A registered vehicle exists whether or not it has ever spoken. Until it has reported
// position, battery and status it cannot be drawn, and it is awaiting a first report rather
// than stale — we have never had it, as opposed to having had it and lost it (F6, §4.8).
func TestAVehicleBecomesReportableOnlyOnceEverySignalHasArrived(t *testing.T) {
	store, projector := freshStore(t)

	if vehicle := store.Snapshot()[0]; vehicle.Reporting || vehicle.Label != "LV-001" {
		t.Fatalf("a freshly registered vehicle is %+v, want known but not reportable", vehicle)
	}

	applyAll(t, projector,
		position(t, vehicleOne, 1, at(1), -115.17, 36.11, 90),
		battery(t, vehicleOne, 1, at(2), 55),
	)
	if store.Snapshot()[0].Reporting {
		t.Error("a vehicle with no reported status is reportable")
	}

	applyAll(t, projector, status(t, vehicleOne, 1, at(3), contract.StatusFree))
	if !store.Snapshot()[0].Reporting {
		t.Error("a vehicle with position, battery and status is not reportable")
	}
}

// Staleness rests on this figure, so what feeds it matters: a replayed roster entry is not
// the vehicle speaking, and a route assignment comes from the assignment system rather than
// from the vehicle. Either would let a silent vehicle look as though it had just reported.
func TestLastObservedFollowsTheVehiclesOwnTelemetryOnly(t *testing.T) {
	store := NewMemoryStore()
	projector, _ := newProjector(t, store)

	applyAll(t, projector,
		record(t, contract.EventVehicleRegistered, vehicleOne, 0, at(600), contract.RegisteredPayload{Label: "LV-001"}),
		position(t, vehicleOne, 1, at(1), -115.17, 36.11, 90),
		battery(t, vehicleOne, 1, at(2), 55),
		status(t, vehicleOne, 1, at(0), contract.StatusEnRoute),
		routeAssigned(t, vehicleOne, 1, at(900), "route-a"),
	)

	if got := store.Snapshot()[0].LastObserved; !got.Equal(at(2)) {
		t.Errorf("LastObserved = %v, want the newest telemetry observation at %v", got, at(2))
	}
}

func freshStore(t *testing.T) (*MemoryStore, *Projector) {
	t.Helper()

	store := NewMemoryStore()
	projector, _ := newProjector(t, store)
	applyAll(t, projector, registration(t, vehicleOne, "LV-001"))
	return store, projector
}

func projectFresh(t *testing.T, registrations, telemetry []Record) []Vehicle {
	t.Helper()

	store := NewMemoryStore()
	projector, _ := newProjector(t, store)
	applyAll(t, projector, registrations...)
	applyAll(t, projector, telemetry...)
	return store.Snapshot()
}

func routeAssigned(t *testing.T, vehicleID string, sequence uint64, observedAt time.Time, routeID string) Record {
	t.Helper()
	return record(t, contract.EventRouteAssigned, vehicleID, sequence, observedAt, contract.Route{
		RouteID:     routeID,
		Geometry:    []contract.Point{{-115.172, 36.114}, {-115.160, 36.130}, {-115.148, 36.163}},
		Destination: contract.Point{-115.148, 36.163},
	})
}

func routeCleared(t *testing.T, vehicleID string, sequence uint64, observedAt time.Time, routeID string) Record {
	t.Helper()
	return record(t, contract.EventRouteCleared, vehicleID, sequence, observedAt, contract.RouteClearedPayload{RouteID: routeID})
}
