package sim

import (
	"log/slog"
	"testing"
	"time"

	"fleetradar/contract"
	"fleetradar/derive"
	"fleetradar/fleet"
)

// The seed is fixed so this verifies the seeded run rather than every run, which is the honest
// claim: the behaviour is probabilistic and pinning it is what makes the assertion stable
// (ADR-0009 consequences).
const (
	seed             = 20260907
	simulatedMinutes = 60
)

type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

// tally projects synchronously and counts what became of every delivery. Synchronously because the
// point is to run an hour of fleet behaviour deterministically; the queue in front of the projection
// has its own test.
type tally struct {
	projector *fleet.Projector
	outcomes  map[fleet.Outcome]int
}

func (t *tally) Consume(r fleet.Record) { t.outcomes[t.projector.Apply(r)]++ }

// This is the only test here whose failure means the submission no longer shows what it claims to
// show. The simulator's parameters are interdependent — drain rate, starting spread, dropout chance
// and assignment rate — so tuning one can make an operator-facing state stop occurring, which is a
// regression with no product code changed and nothing else in the suite would catch it
// (ADR-0009 §9.6).
func TestTheDemoStillDemonstrates(t *testing.T) {
	clock := &clock{at: time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)}
	store := fleet.NewMemoryStore()
	discard := slog.New(slog.DiscardHandler)

	counted := &tally{
		projector: fleet.NewProjector(store, discard, clock.now),
		outcomes:  map[fleet.Outcome]int{},
	}
	simulator := New(counted, discard, clock.now, seed)

	simulator.Replay()

	var (
		statusesSeen    = map[contract.VehicleStatus]bool{}
		wasLow          = map[string]bool{}
		crossedIntoLow  bool
		staleSeen       bool
		leftServiceArea bool
		awaitingSeen    bool
		routesSeen      bool
		enRouteHigh     int
		staleHigh       int
		lowHigh         int
	)
	area := contract.ServiceArea()

	for tick := range simulatedMinutes * 60 * ticksPerReport {
		simulator.Step()
		clock.at = clock.at.Add(TickInterval)

		// Derived once a simulated second rather than every tick. Deriving the whole fleet 72,000
		// times would say nothing more and would make this test slow enough to skip.
		if tick%ticksPerReport != 0 {
			continue
		}

		vehicles := store.Snapshot()
		snapshot := derive.Snapshot(vehicles, contract.Zones(), contract.LifecycleReady, clock.now())

		enRouteHigh = max(enRouteHigh, snapshot.Summary.EnRoute)
		staleHigh = max(staleHigh, snapshot.Summary.Stale)
		lowHigh = max(lowHigh, snapshot.Summary.LowBattery)
		awaitingSeen = awaitingSeen || snapshot.Summary.AwaitingFirstReport > 0
		staleSeen = staleSeen || snapshot.Summary.Stale > 0
		routesSeen = routesSeen || len(derive.Routes(vehicles).Routes) > 0

		for _, vehicle := range snapshot.Vehicles {
			statusesSeen[vehicle.Status] = true
			leftServiceArea = leftServiceArea || !contract.Contains(area, vehicle.Position)

			low := vehicle.BatteryPercent < contract.LowBatteryPercent
			crossedIntoLow = crossedIntoLow || (low && wasLow[vehicle.VehicleID] == false && vehicle.SilentForMs < contract.StaleAfter.Milliseconds())
			wasLow[vehicle.VehicleID] = low
		}
	}

	// Reported so that anyone retuning the parameters can see what the run actually contained,
	// rather than only whether it passed.
	t.Logf("over %d simulated minutes: at most %d remotely driven, %d stale, %d low on energy",
		simulatedMinutes, enRouteHigh, staleHigh, lowHigh)

	for _, status := range []contract.VehicleStatus{contract.StatusFree, contract.StatusEnRoute, contract.StatusWithCustomer} {
		if !statusesSeen[status] {
			t.Errorf("no vehicle was ever %s", status)
		}
	}
	if !routesSeen {
		t.Error("no route was ever drawn, so R3 is undemonstrable")
	}
	if !staleSeen {
		t.Error("no vehicle ever went stale, so the freshness feature is undemonstrable")
	}
	if !crossedIntoLow {
		t.Error("no vehicle ever crossed the energy threshold, so F5 is undemonstrable")
	}
	if !leftServiceArea {
		t.Error("no vehicle ever left the service area, so a specified behaviour is undemonstrable")
	}
	if !awaitingSeen {
		t.Error("no vehicle was ever awaiting a first report, so that state is undemonstrable")
	}
	if enRouteHigh < EnRouteTarget-1 || enRouteHigh > EnRouteTarget+2 {
		t.Errorf("remotely-driven vehicles peaked at %d, want the scheduler to hold about %d", enRouteHigh, EnRouteTarget)
	}
}

// The producer's obligations, checked from the consumer's side. Duplicates and superseded
// observations must actually occur, or at-least-once and unordered delivery are handled only in
// theory; nothing may be uninterpretable, because a log permanently carrying warnings makes a
// healthy system look broken; and nothing may arrive for an unregistered vehicle, because replay
// completes before telemetry begins (ADR-0003 §3.9, ADR-0007 §7.7, §7.8).
func TestTheSourceHonoursItsObligations(t *testing.T) {
	clock := &clock{at: time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)}
	discard := slog.New(slog.DiscardHandler)

	counted := &tally{
		projector: fleet.NewProjector(fleet.NewMemoryStore(), discard, clock.now),
		outcomes:  map[fleet.Outcome]int{},
	}
	simulator := New(counted, discard, clock.now, seed)

	simulator.Replay()
	for range 5 * 60 * ticksPerReport {
		simulator.Step()
		clock.at = clock.at.Add(TickInterval)
	}

	if counted.outcomes[fleet.Duplicate] == 0 {
		t.Error("no delivery was ever duplicated, so at-least-once handling is never exercised")
	}
	if counted.outcomes[fleet.Superseded] == 0 {
		t.Error("no delivery ever arrived out of order, so unordered handling is never exercised")
	}
	if got := counted.outcomes[fleet.Uninterpretable]; got != 0 {
		t.Errorf("%d deliveries were uninterpretable, want none in a normal run", got)
	}
	if got := counted.outcomes[fleet.UnknownVehicle]; got != 0 {
		t.Errorf("%d deliveries arrived for an unregistered vehicle, want none: replay precedes telemetry", got)
	}
}
