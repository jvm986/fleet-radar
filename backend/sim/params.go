package sim

import (
	"time"

	"fleetradar/contract"
)

// Every tunable of the simulation is a constant here: the simulation is hidden from the client,
// not from the code (ADR-0007 §7.13).
//
// They are interdependent, and that is the risk worth naming. Drain rate, starting battery
// spread, dropout chance and assignment rate all have to be jointly plausible, and a change to
// one can stop an operator-facing state occurring at all — a regression in the submission with no
// product code changed. That is what the demo test guards (ADR-0009 §9.6).
const (
	// FleetSize is the brief's ~100 vehicles, and the one value to change for the 1000-vehicle
	// run (ADR-0008 §8.10).
	FleetSize = 100

	// EnRouteTarget is the brief's ~10 simultaneously remotely-driven vehicles. A scheduler holds
	// the count here rather than each vehicle rolling a dice, which would give ten on average
	// with visible excursions instead of an invariant (ADR-0007 §7.5).
	EnRouteTarget = 10

	// PickupShare is how many dispatched journeys are going to fetch a customer rather than to
	// reposition an idle vehicle. It sets how much of the fleet is WITH_CUSTOMER at any time.
	PickupShare = 0.7

	// TickInterval is the simulation's own tick, finer than the reporting interval so that
	// vehicles can be staggered across it. That produces a smooth ~100 events per second rather
	// than 100 in an instant and then silence, so ingest under burst stays an edge case rather
	// than the normal one (ADR-0007 §7.14).
	TickInterval = 50 * time.Millisecond

	// ticksPerReport is how many ticks make up one reporting interval, and so how many distinct
	// phases a vehicle can be given.
	ticksPerReport = int(contract.ReportingInterval / TickInterval)

	// batteryEveryNReports keeps battery at roughly ten seconds. It moves far more slowly than
	// position, and reporting it at 1 Hz would be repetition (ADR-0003 §3.13).
	batteryEveryNReports = 10

	// Speed is a plausible average for city driving including junctions, in metres per second.
	// At 1 Hz reporting it puts about 15 metres between consecutive positions, which is a few
	// pixels at working zoom — small enough that rendering positions as reported reads as
	// movement rather than as teleporting (ADR-0002 §2.5).
	Speed = 15.0

	// TripMinMetres and TripMaxMetres bound how far a journey goes. Long enough to be a journey
	// across a city, short enough that a reviewer sees several complete.
	TripMinMetres = 2_000.0
	TripMaxMetres = 6_000.0

	// StartingBatteryMin and StartingBatteryMax spread the fleet's energy at startup. The spread
	// is what makes low battery emerge rather than needing a mechanism: some vehicles begin just
	// above the threshold and cross it within minutes (ADR-0007 §7.7).
	StartingBatteryMin = 18.0
	StartingBatteryMax = 100.0

	// DrainPerKm is close to a real EV's consumption: roughly 0.15 kWh/km against a 60 kWh pack.
	DrainPerKm = 0.3
	// IdleDrainPerMinute keeps a parked vehicle's energy moving, so crossing the threshold is not
	// something only driving vehicles do.
	IdleDrainPerMinute = 0.15

	// ChargeFloor, ChargeTarget and ChargePerMinute are the energy recovery that exists because
	// without it the fleet dies: charging locations are out of scope, so drain alone means every
	// vehicle reaches zero and a demo left running ends with a dead fleet.
	//
	// A vehicle below the floor while parked stays parked and its energy rises until the target.
	// It remains FREE throughout, because no charging status exists and the spec excludes
	// inventing one — the operator sees a battery increase, which is honest, since a parked
	// vehicle being charged is exactly what would be happening (ADR-0007 §7.6).
	ChargeFloor     = 5.0
	ChargeTarget    = 80.0
	ChargePerMinute = 3.0

	// DispatchMinimumBattery keeps the scheduler from sending out a vehicle that cannot finish.
	// This is the simulated dispatcher's judgement, not the operator's view: a low-energy
	// available vehicle still counts as coverage, because the threshold is a cue to plan a charge
	// rather than a declaration that the vehicle cannot do a job (PRODUCT-SPEC §2.5).
	DispatchMinimumBattery = 25.0

	// LeavesServiceAreaChance is how often a customer drives out of the service area rather than to
	// somewhere within it. A customer may go anywhere and the map must not lie about it, but it has to
	// actually happen or a specified behaviour is undemonstrable (ADR-0007 §7.12).
	//
	// Higher than realism alone would suggest, and tuned to the point where raising it further changes
	// nothing: the wait is not the attempt rate but the drive. The boundary is roughly fifteen
	// kilometres from where most customers are picked up, so leaving the area takes about a quarter of
	// an hour at city speed whatever this number says. Measured: 0.2 gives a first departure at 26
	// minutes, 0.4 at 15, and 0.6 also at 15.
	LeavesServiceAreaChance = 0.4

	// ReassignChancePerReport re-plans a journey in progress. The operator needs the current
	// intent rather than the original, and without this the route-update path would never run in
	// a live system (PRODUCT-SPEC F2, §2.2).
	ReassignChancePerReport = 0.004

	// SilenceChance, SilenceMin and SilenceMax are the dropouts. Staleness has to be modelled
	// explicitly because nothing about the simulated world causes silence, and it is tuned so
	// that one to three vehicles are quiet at any moment out of a hundred (ADR-0007 §7.7).
	SilenceChance = 0.0012
	SilenceMin    = 5 * time.Second
	SilenceMax    = 30 * time.Second

	// DuplicateChance and the delay window make delivery imperfect, so that at-least-once and
	// unordered handling are exercised by the running system rather than only asserted in tests.
	//
	// The delay is expressed relative to the reporting interval, and has to exceed it. ADR-0007 §7.8
	// specified a flat 200–400 ms, which cannot reorder anything: the next report of the same signal
	// is a whole interval away, so a delayed event still arrives before the observation that would
	// supersede it, is applied as the newest, and demonstrates nothing. A delay longer than the
	// interval is what makes a late event genuinely late.
	//
	// It stays safe for staleness for the reason §7.8 gives: staleness compares the clock against
	// the newest observation received, and only 2% of events are held, so a newer observation still
	// lands on time. The late one arrives, is discarded as superseded, and staleness is untouched.
	// Only delaying every event would trip it.
	DuplicateChance = 0.02
	DelayChance     = 0.02
	DelayMin        = contract.ReportingInterval * 3 / 2
	DelayMax        = contract.ReportingInterval * 5 / 2
)
