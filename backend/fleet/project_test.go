package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"fleetradar/contract"
)

const (
	vehicleOne = "b6a1d0c4-2f77-4a1e-bb45-2c9f6d3a1e80"
	vehicleTwo = "1d4f9a52-6c18-4b03-8f6d-90a2e7c41b55"
)

var epoch = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func at(seconds int) time.Time { return epoch.Add(time.Duration(seconds) * time.Second) }

// record serialises an event the way the source will, because bytes are what cross into
// ingest — so these tests exercise decoding and validation rather than bypassing them
// (ADR-0007 §7.2). The event id is derived from the event so that a duplicate delivery is
// byte-identical to the original.
func record(t *testing.T, eventType contract.EventType, vehicleID string, sequence uint64, observedAt time.Time, payload any) Record {
	t.Helper()

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	body, err := json.Marshal(contract.Envelope{
		EventID:    fmt.Sprintf("%s-%s-%d", vehicleID, eventType, sequence),
		Type:       eventType,
		VehicleID:  vehicleID,
		Sequence:   sequence,
		ObservedAt: observedAt,
		Payload:    encodedPayload,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	topic, known := eventType.Topic()
	if !known {
		topic = contract.TopicTelemetry
	}
	return Record{Topic: topic, Key: vehicleID, Payload: body}
}

func registration(t *testing.T, vehicleID, label string) Record {
	t.Helper()
	return record(t, contract.EventVehicleRegistered, vehicleID, 0, at(0), contract.RegisteredPayload{Label: label})
}

func position(t *testing.T, vehicleID string, sequence uint64, observedAt time.Time, lng, lat, heading float64) Record {
	t.Helper()
	return record(t, contract.EventVehiclePosition, vehicleID, sequence, observedAt, contract.PositionPayload{
		Position: contract.Point{lng, lat},
		Heading:  heading,
	})
}

func battery(t *testing.T, vehicleID string, sequence uint64, observedAt time.Time, percent float64) Record {
	t.Helper()
	return record(t, contract.EventVehicleBattery, vehicleID, sequence, observedAt, contract.BatteryPayload{Percent: percent})
}

func status(t *testing.T, vehicleID string, sequence uint64, observedAt time.Time, s contract.VehicleStatus) Record {
	t.Helper()
	return record(t, contract.EventVehicleStatus, vehicleID, sequence, observedAt, contract.StatusPayload{Status: s})
}

// newProjector clocks well past every observation used here, so the implausible-timestamp
// warning stays out of the way of tests that are about something else.
func newProjector(t *testing.T, store FleetStore) (*Projector, *logCapture) {
	t.Helper()
	captured := &logCapture{}
	return NewProjector(store, slog.New(captured), func() time.Time { return at(3600) }), captured
}

func applyAll(t *testing.T, projector *Projector, records ...Record) {
	t.Helper()
	for _, r := range records {
		projector.Apply(r)
	}
}

// A delivery that cannot be read must be discarded with a reason and must never stop the
// fleet. Nothing in a normal run produces one — the simulator emits no malformed events,
// because a log permanently carrying warnings makes a healthy system look broken — so this
// path exists only here (ADR-0003 §3.11, ADR-0007 §7.8).
func TestUninterpretableDeliveriesAreDiscarded(t *testing.T) {
	valid := func(t *testing.T) Record {
		return position(t, vehicleOne, 1, at(1), -115.17, 36.11, 90)
	}

	for _, tc := range []struct {
		name    string
		deliver func(t *testing.T) Record
	}{
		{"not JSON", func(t *testing.T) Record {
			r := valid(t)
			r.Payload = []byte(`{"type":`)
			return r
		}},
		{"unknown event type", func(t *testing.T) Record {
			return record(t, contract.EventType("VehicleRetired"), vehicleOne, 1, at(1), struct{}{})
		}},
		{"right type, wrong topic", func(t *testing.T) Record {
			r := valid(t)
			r.Topic = contract.TopicLifecycle
			return r
		}},
		{"partition key is not the vehicle", func(t *testing.T) Record {
			r := valid(t)
			r.Key = vehicleTwo
			return r
		}},
		{"no vehicle", func(t *testing.T) Record {
			return position(t, "", 1, at(1), -115.17, 36.11, 90)
		}},
		{"no observation timestamp", func(t *testing.T) Record {
			return position(t, vehicleOne, 1, time.Time{}, -115.17, 36.11, 90)
		}},
		{"position is not a coordinate", func(t *testing.T) Record {
			return position(t, vehicleOne, 1, at(1), -115.17, 236.11, 90)
		}},
		{"heading is not a bearing", func(t *testing.T) Record {
			return position(t, vehicleOne, 1, at(1), -115.17, 36.11, 360)
		}},
		{"battery is not a proportion", func(t *testing.T) Record {
			return battery(t, vehicleOne, 1, at(1), 150)
		}},
		{"status is not one of the three", func(t *testing.T) Record {
			return status(t, vehicleOne, 1, at(1), contract.VehicleStatus("CHARGING"))
		}},
		{"registration without a label", func(t *testing.T) Record {
			return record(t, contract.EventVehicleRegistered, vehicleOne, 0, at(0), contract.RegisteredPayload{})
		}},
		{"route without a path", func(t *testing.T) Record {
			return record(t, contract.EventRouteAssigned, vehicleOne, 1, at(1), contract.Route{
				RouteID:     "r-1",
				Geometry:    []contract.Point{{-115.17, 36.11}},
				Destination: contract.Point{-115.17, 36.11},
			})
		}},
		{"route cleared without an id", func(t *testing.T) Record {
			return record(t, contract.EventRouteCleared, vehicleOne, 1, at(1), contract.RouteClearedPayload{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewMemoryStore()
			projector, _ := newProjector(t, store)
			applyAll(t, projector, registration(t, vehicleOne, "LV-001"))

			if got := projector.Apply(tc.deliver(t)); got != Uninterpretable {
				t.Errorf("Apply() = %q, want %q", got, Uninterpretable)
			}
			if reporting := store.Snapshot()[0].Reporting; reporting {
				t.Error("a discarded delivery changed the vehicle's state")
			}
		})
	}
}

// The observation clock is authoritative even when it is wrong, because the operator's
// question is how current the information about the vehicle is. A producer running ahead is
// logged rather than corrected, since it would otherwise make staleness unreachable and
// nothing would say so (ADR-0003 §3.5).
func TestFutureObservationIsAppliedAndWarnedAbout(t *testing.T) {
	store := NewMemoryStore()
	projector, captured := newProjector(t, store)

	applyAll(t, projector, registration(t, vehicleOne, "LV-001"))
	if got := projector.Apply(position(t, vehicleOne, 1, at(7200), -115.17, 36.11, 90)); got != Applied {
		t.Fatalf("Apply() = %q, want %q", got, Applied)
	}
	if captured.count(slog.LevelWarn, "observation timestamp is in the future") != 1 {
		t.Error("a future observation timestamp was not warned about")
	}
}

// The log is the whole of the observability, and discards are the only place at-least-once
// and unordered delivery are observable at all. So the levels are pinned: the designed
// outcomes stay visible by default without presenting a healthy system as a failing one
// (ADR-0009 §9.7, ADR-0010 §10.1).
func TestDiscardsAreVisibleInTheLog(t *testing.T) {
	store := NewMemoryStore()
	projector, captured := newProjector(t, store)

	applyAll(t, projector,
		registration(t, vehicleOne, "LV-001"),
		position(t, vehicleOne, 4, at(4), -115.17, 36.11, 90),
		position(t, vehicleOne, 4, at(4), -115.17, 36.11, 90),
		position(t, vehicleOne, 3, at(3), -115.18, 36.12, 90),
		position(t, vehicleTwo, 1, at(1), -115.19, 36.13, 90),
	)

	if got := captured.count(slog.LevelInfo, "discarded"); got != 2 {
		t.Errorf("discards logged at info = %d, want 2 (one duplicate, one superseded)", got)
	}
	if got := captured.count(slog.LevelWarn, "discarded"); got != 1 {
		t.Errorf("discards logged at warning = %d, want 1 (telemetry for an unregistered vehicle)", got)
	}
}

type logCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}

func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) count(level slog.Level, message string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	found := 0
	for _, r := range c.records {
		if r.Level == level && r.Message == message {
			found++
		}
	}
	return found
}
