package contract

import (
	"encoding/json"
	"testing"
	"time"
)

// The signal mapping is the kind of thing that fails silently. If RouteCleared were given a
// register of its own, a clear arriving out of order would be compared against nothing and
// could wipe a route assigned after it — and nothing on screen would say so.
func TestEventTypeSignal(t *testing.T) {
	for _, tc := range []struct {
		eventType EventType
		signal    Signal
		known     bool
	}{
		{EventVehicleRegistered, SignalRegistration, true},
		{EventVehiclePosition, SignalPosition, true},
		{EventVehicleBattery, SignalBattery, true},
		{EventVehicleStatus, SignalStatus, true},
		{EventRouteAssigned, SignalRoute, true},
		{EventRouteCleared, SignalRoute, true},
		{EventType("VehicleRetired"), "", false},
		{EventType(""), "", false},
	} {
		t.Run(string(tc.eventType), func(t *testing.T) {
			signal, known := tc.eventType.Signal()
			if signal != tc.signal || known != tc.known {
				t.Errorf("Signal() = %q, %v; want %q, %v", signal, known, tc.signal, tc.known)
			}
		})
	}
}

// Coordinates are longitude first. Nothing on screen distinguishes a transposed coordinate
// from a vehicle in the wrong place, so the wire order is pinned here.
func TestEnvelopeDecodesPosition(t *testing.T) {
	raw := []byte(`{
		"eventId": "9f1c8e10-0d2a-4f2b-9c31-6f1b0a7d2e44",
		"type": "VehiclePosition",
		"vehicleId": "b6a1d0c4-2f77-4a1e-bb45-2c9f6d3a1e80",
		"sequence": 41,
		"observedAt": "2026-09-07T12:00:00Z",
		"payload": {"position": [-115.1728, 36.1147], "heading": 275.5}
	}`)

	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != EventVehiclePosition {
		t.Errorf("Type = %q, want %q", envelope.Type, EventVehiclePosition)
	}
	if envelope.Sequence != 41 {
		t.Errorf("Sequence = %d, want 41", envelope.Sequence)
	}
	if want := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC); !envelope.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %v, want %v", envelope.ObservedAt, want)
	}

	var payload PositionPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got := payload.Position.Lng(); got != -115.1728 {
		t.Errorf("Lng() = %v, want -115.1728", got)
	}
	if got := payload.Position.Lat(); got != 36.1147 {
		t.Errorf("Lat() = %v, want 36.1147", got)
	}
	if payload.Heading != 275.5 {
		t.Errorf("Heading = %v, want 275.5", payload.Heading)
	}
}
