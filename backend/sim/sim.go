// Package sim is the event source. It stands in for a Kafka broker, and it is deliberately not a
// toy: every earlier decision placed an obligation on the producer, and they all come due here —
// a sequence number per vehicle per signal, absolute values only, registration replay before
// telemetry, and a regular cadence (ADR-0003 consequences, ADR-0007).
//
// Serialised bytes cross into ingest rather than Go structs. Passing structs would be faster and
// shorter, and it would mean validation and the discard path never executed in the running system —
// making the seam fictional exactly where the brief is looking (ADR-0007 §7.2).
package sim

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"fleetradar/contract"
	"fleetradar/fleet"
)

type Simulator struct {
	graph    *Graph
	delivery *delivery
	log      *slog.Logger
	// now is injected so that a test can run half an hour of fleet behaviour in a moment
	// (ADR-0009 §9.12).
	now      func() time.Time
	random   *rand.Rand
	vehicles []*vehicle
	tick     int
}

// New builds the fleet. The seed is logged by the caller: a simulation that cannot be reproduced
// cannot be debugged, and the cost of being able to is one log line (ADR-0007 §7.11).
func New(consumer fleet.Consumer, log *slog.Logger, now func() time.Time, seed uint64) *Simulator {
	random := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	graph := mustLoadGraph(networkJSON)

	simulator := &Simulator{
		graph:    graph,
		delivery: &delivery{consumer: consumer, random: random},
		log:      log,
		now:      now,
		random:   random,
	}
	for i := range FleetSize {
		node := graph.RandomNode(random)
		simulator.vehicles = append(simulator.vehicles, &vehicle{
			id: uuid(random),
			// Four digits because the label has to be sized for the fleet from the start: a
			// three-digit label runs out at exactly 1000, the number the brief asks about, and
			// would need re-badging at the worst moment (PRODUCT-SPEC §7.3).
			label: fmt.Sprintf("LV-%04d", i+1),
			node:  node,
			at:    graph.Position(node),
			// A parked vehicle's heading is meaningful — it is which way the vehicle will leave —
			// so it has one from the start (PRODUCT-SPEC §2.1).
			heading:   random.Float64() * 360,
			battery:   StartingBatteryMin + random.Float64()*(StartingBatteryMax-StartingBatteryMin),
			slot:      i % ticksPerReport,
			mute:      i == 0,
			sequences: make(map[contract.Signal]uint64),
		})
	}
	return simulator
}

// Replay emits every registration, which is what a compacted lifecycle topic delivers on subscribe.
// It has to complete before telemetry begins, or telemetry for vehicles the backend has not learned
// about yet is discarded at startup for no reason (ADR-0003 §3.9, §3.12).
func (s *Simulator) Replay() {
	now := s.now()
	for _, vehicle := range s.vehicles {
		s.delivery.direct(s.record(now, vehicle, event{
			eventType: contract.EventVehicleRegistered,
			payload:   contract.RegisteredPayload{Label: vehicle.label},
		}))
	}
	s.log.Info("replayed the fleet roster",
		slog.Int("vehicles", len(s.vehicles)),
		slog.Int("intersections", len(s.graph.ids)),
		slog.Int("intersectionsOutsideServiceArea", len(s.graph.Outside())),
	)
}

// Run drives the simulation until the context ends.
func (s *Simulator) Run(ctx context.Context) {
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Step()
		}
	}
}

// Step advances the simulation by one tick. It is exported so that a test can drive the whole
// pipeline against a fake clock rather than waiting on real time.
func (s *Simulator) Step() {
	now := s.now()
	s.delivery.release(now)

	for _, vehicle := range s.vehicles {
		s.emit(now, vehicle, vehicle.advance(TickInterval, s.graph, s.random))
	}
	s.dispatch(now)

	// Only the vehicles whose turn it is speak, which spreads the fleet's reports evenly across the
	// interval instead of bursting all of them at once (ADR-0007 §7.14).
	for _, vehicle := range s.vehicles {
		if vehicle.slot != s.tick%ticksPerReport {
			continue
		}
		s.emit(now, vehicle, vehicle.replan(s.graph, s.random))
		s.emit(now, vehicle, vehicle.report(now, s.random))
	}
	s.tick++
}

// dispatch holds the remotely-driven count at the target, which makes the brief's "~10 EN_ROUTE" an
// invariant rather than an average with visible excursions (ADR-0007 §7.5).
//
// It starts from a random position in the fleet so the work spreads across it, rather than the same
// few vehicles taking every job while the rest sit still all run.
func (s *Simulator) dispatch(now time.Time) {
	enRoute := 0
	for _, vehicle := range s.vehicles {
		if vehicle.leg.status() == contract.StatusEnRoute {
			enRoute++
		}
	}

	start := s.random.IntN(len(s.vehicles))
	for i := range s.vehicles {
		if enRoute >= EnRouteTarget {
			return
		}

		vehicle := s.vehicles[(start+i)%len(s.vehicles)]
		// A dispatcher would not send out a vehicle that cannot finish, nor unplug one that is
		// charging. Neither is a claim about what the operator sees: a low-energy available vehicle
		// still counts as coverage (PRODUCT-SPEC §2.5).
		if vehicle.leg != legParked || vehicle.charging || vehicle.battery < DispatchMinimumBattery {
			continue
		}

		// Either fetching a customer or repositioning. Both are a remote driver on a planned
		// route, which is what EN_ROUTE means (PRODUCT-SPEC §6.1.1).
		purpose := legToCustomer
		if s.random.Float64() >= PickupShare {
			purpose = legToParking
		}

		events := vehicle.dispatchTo(purpose, s.graph, s.random)
		if len(events) == 0 {
			continue
		}
		s.emit(now, vehicle, events)
		enRoute++
	}
}

func (s *Simulator) emit(now time.Time, vehicle *vehicle, events []event) {
	for _, produced := range events {
		s.delivery.send(now, s.record(now, vehicle, produced))
	}
}

// record serialises one event the way a broker would deliver it: a topic, the vehicle identity as the
// partition key, and a body of bytes. The event type decides which topic it belongs on and which of the
// vehicle's counters numbers it, so neither is restated by the code that produced it.
func (s *Simulator) record(now time.Time, vehicle *vehicle, produced event) fleet.Record {
	topic, known := produced.eventType.Topic()
	if !known {
		panic(fmt.Sprintf("sim: %s belongs on no topic", produced.eventType))
	}
	signal, known := produced.eventType.Signal()
	if !known {
		panic(fmt.Sprintf("sim: %s writes to no signal", produced.eventType))
	}

	payload, err := json.Marshal(produced.payload)
	if err != nil {
		panic(fmt.Sprintf("sim: %s payload will not serialise: %v", produced.eventType, err))
	}

	body, err := json.Marshal(contract.Envelope{
		EventID:   uuid(s.random),
		Type:      produced.eventType,
		VehicleID: vehicle.id,
		Sequence:  vehicle.next(signal),
		// The observation clock, which is authoritative all the way to the operator's screen
		// (ADR-0003 §3.5).
		ObservedAt: now,
		Payload:    payload,
	})
	if err != nil {
		panic(fmt.Sprintf("sim: %s envelope will not serialise: %v", produced.eventType, err))
	}

	return fleet.Record{Topic: topic, Key: vehicle.id, Payload: body}
}

// uuid formats sixteen bytes from the seeded source as a version 4 UUID, so a run is reproducible
// down to its identifiers and no dependency is needed for eight lines of formatting.
func uuid(random *rand.Rand) string {
	var bytes [16]byte
	binary.BigEndian.PutUint64(bytes[0:8], random.Uint64())
	binary.BigEndian.PutUint64(bytes[8:16], random.Uint64())
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}
