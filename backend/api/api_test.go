package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fleetradar/contract"
	"fleetradar/fleet"
)

const vehicleOne = "b6a1d0c4-2f77-4a1e-bb45-2c9f6d3a1e80"

var published = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func newPublisher(t *testing.T) (*fleet.Projector, *Publisher) {
	t.Helper()

	discard := slog.New(slog.DiscardHandler)
	store := fleet.NewMemoryStore()
	clock := func() time.Time { return published }
	return fleet.NewProjector(store, discard, clock), NewPublisher(store, discard, clock)
}

// record serialises an event the way the source does, because the read path is only worth testing
// against state that arrived the way real state arrives.
func record(t *testing.T, eventType contract.EventType, sequence uint64, payload any) fleet.Record {
	t.Helper()

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	body, err := json.Marshal(contract.Envelope{
		EventID:    fmt.Sprintf("%s-%d", eventType, sequence),
		Type:       eventType,
		VehicleID:  vehicleOne,
		Sequence:   sequence,
		ObservedAt: published,
		Payload:    encoded,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	topic, _ := eventType.Topic()
	return fleet.Record{Topic: topic, Key: vehicleOne, Payload: body}
}

func reportingVehicle(t *testing.T, projector *fleet.Projector, status contract.VehicleStatus) {
	t.Helper()

	for _, r := range []fleet.Record{
		record(t, contract.EventVehicleRegistered, 1, contract.RegisteredPayload{Label: "LV-0001"}),
		record(t, contract.EventVehiclePosition, 1, contract.PositionPayload{Position: contract.Point{-115.172, 36.114}, Heading: 90}),
		record(t, contract.EventVehicleBattery, 1, contract.BatteryPayload{Percent: 76}),
		record(t, contract.EventVehicleStatus, 1, contract.StatusPayload{Status: status}),
	} {
		projector.Apply(r)
	}
}

func routeAssigned(t *testing.T, projector *fleet.Projector, sequence uint64, routeID string) {
	t.Helper()

	projector.Apply(record(t, contract.EventRouteAssigned, sequence, contract.Route{
		RouteID:     routeID,
		Geometry:    []contract.Point{{-115.172, 36.114}, {-115.160, 36.130}},
		Destination: contract.Point{-115.160, 36.130},
	}))
}

// message is one Server-Sent Event: the kind, and the payload.
type message struct {
	event string
	data  []byte
}

func parse(t *testing.T, frame []byte) message {
	t.Helper()

	var sent message
	for line := range strings.SplitSeq(string(frame), "\n") {
		switch {
		case strings.HasPrefix(line, "event: "):
			sent.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			sent.data = []byte(strings.TrimPrefix(line, "data: "))
		}
	}
	if sent.event == "" {
		t.Fatalf("frame carries no event name: %q", frame)
	}
	return sent
}

// take is what the viewer would have read. Nothing pending is a failure rather than a wait, because
// publication is synchronous with Publish and a test that waited would be testing the scheduler.
func take(t *testing.T, slot chan []byte) message {
	t.Helper()

	select {
	case frame := <-slot:
		return parse(t, frame)
	default:
		t.Fatal("nothing was published")
		return message{}
	}
}

func nothingPending(t *testing.T, slot chan []byte) {
	t.Helper()

	select {
	case frame := <-slot:
		t.Fatalf("unexpected frame: %s", frame)
	default:
	}
}

func snapshotFrom(t *testing.T, sent message) contract.Snapshot {
	t.Helper()

	if sent.event != contract.MessageSnapshot {
		t.Fatalf("event = %q, want %q", sent.event, contract.MessageSnapshot)
	}
	var snapshot contract.Snapshot
	if err := json.Unmarshal(sent.data, &snapshot); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot
}

// The opening sequence, over a real connection. Config first, so it is present before any snapshot
// needs it; then the geometry of the routes already on the map; then snapshots. The client renders its
// legend from what it is sent and holds no copy of any threshold, which is what keeps the legend from
// contradicting the logic being applied (ADR-0001 §1.8, ADR-0005 §5.8).
func TestAViewerIsSentConfigThenGeometryThenSnapshots(t *testing.T) {
	projector, publisher := newPublisher(t)

	reportingVehicle(t, projector, contract.StatusEnRoute)
	routeAssigned(t, projector, 1, "route-1")
	publisher.MarkReady()
	publisher.Publish()

	server := httptest.NewServer(http.HandlerFunc(publisher.Stream))
	defer server.Close()

	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	// The opening frames are written before the handler waits on anything, so they are already there;
	// the snapshot follows the next publication. Reading more than that would be racing the
	// depth-one buffer, whose whole purpose is to drop what a viewer has not taken.
	reader := bufio.NewReader(response.Body)
	publisher.Publish()

	var opening []message
	for len(opening) < 3 {
		frame, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the stream: %v", err)
		}
		if strings.HasPrefix(frame, "event: ") {
			body, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("reading the stream: %v", err)
			}
			opening = append(opening, parse(t, []byte(frame+body)))
		}
	}

	want := []string{contract.MessageConfig, contract.MessageRoutes, contract.MessageSnapshot}
	for i, event := range want {
		if opening[i].event != event {
			t.Fatalf("the stream opened with %q, %q, %q; want %v", opening[0].event, opening[1].event, opening[2].event, want)
		}
	}

	var config contract.Config
	if err := json.Unmarshal(opening[0].data, &config); err != nil {
		t.Fatalf("config: %v", err)
	}
	switch {
	case config.Thresholds.LowBatteryPercent != contract.LowBatteryPercent:
		t.Errorf("config states %v as the low-energy threshold, want %v", config.Thresholds.LowBatteryPercent, contract.LowBatteryPercent)
	case config.Thresholds.StaleAfterMs != int(contract.StaleAfter.Milliseconds()):
		t.Errorf("config states %dms as the silence that counts as stale, want %dms", config.Thresholds.StaleAfterMs, contract.StaleAfter.Milliseconds())
	case config.TickIntervalMs != int(contract.TickInterval.Milliseconds()):
		t.Errorf("config states a %dms tick, want %dms", config.TickIntervalMs, contract.TickInterval.Milliseconds())
	case !bytes.Contains(config.ServiceArea, []byte("zoneId")):
		t.Error("config carries no zone geometry, so the client has nothing to draw")
	}

	var routes contract.Routes
	if err := json.Unmarshal(opening[1].data, &routes); err != nil {
		t.Fatalf("routes: %v", err)
	}
	if len(routes.Routes) != 1 || routes.Routes[0].RouteID != "route-1" {
		t.Errorf("routes = %+v, want the one being driven", routes.Routes)
	}

	if vehicles := snapshotFrom(t, opening[2]).Vehicles; len(vehicles) != 1 || vehicles[0].RouteID != "route-1" {
		t.Errorf("vehicles = %+v, want the one vehicle, referencing its route", vehicles)
	}
}

// Geometry is sent on connect and when it changes, and not otherwise. Carried in every snapshot it
// would dominate the payload, since a polyline lasts minutes while snapshots go out five times a
// second (ADR-0005 §5.5).
func TestGeometryIsSentOnlyWhenItChanges(t *testing.T) {
	projector, publisher := newPublisher(t)
	reportingVehicle(t, projector, contract.StatusEnRoute)
	routeAssigned(t, projector, 1, "route-1")
	publisher.MarkReady()

	viewer, _ := publisher.subscribe()

	publisher.Publish()
	if sent := take(t, viewer.routes); sent.event != contract.MessageRoutes {
		t.Fatalf("event = %q, want %q", sent.event, contract.MessageRoutes)
	}
	take(t, viewer.snapshots)

	publisher.Publish()
	nothingPending(t, viewer.routes)
	take(t, viewer.snapshots)

	// A revised journey is assigned a new route id, which is what makes comparing ids sufficient.
	routeAssigned(t, projector, 2, "route-2")
	publisher.Publish()
	take(t, viewer.routes)
}

// Before replay has completed, the fleet on screen is not the whole fleet, and the operator has to be
// told so rather than shown a partial fleet as though it were complete. The claim travels inside the
// snapshot, so there is no window in which a viewer holds data but not the claim about that data
// (ADR-0004 §4.11, ADR-0005 §5.12).
func TestTheSnapshotSaysWhetherTheFleetIsStillFilling(t *testing.T) {
	projector, publisher := newPublisher(t)
	reportingVehicle(t, projector, contract.StatusFree)
	viewer, _ := publisher.subscribe()

	publisher.Publish()
	if got := snapshotFrom(t, take(t, viewer.snapshots)).Lifecycle; got != contract.LifecycleStarting {
		t.Errorf("Lifecycle = %q, want %q while the roster is still arriving", got, contract.LifecycleStarting)
	}

	publisher.MarkReady()
	publisher.Publish()
	if got := snapshotFrom(t, take(t, viewer.snapshots)).Lifecycle; got != contract.LifecycleReady {
		t.Errorf("Lifecycle = %q, want %q once replay has completed", got, contract.LifecycleReady)
	}
}

// An empty fleet is one of the four ways of knowing nothing, and it is not any of the others:
// connected, current, and there genuinely are no vehicles (PRODUCT-SPEC F6, ADR-0009 §9.9).
func TestAnEmptyFleetIsPublishedAsEmptyRatherThanNotAtAll(t *testing.T) {
	_, publisher := newPublisher(t)
	publisher.MarkReady()
	viewer, _ := publisher.subscribe()

	publisher.Publish()
	snapshot := snapshotFrom(t, take(t, viewer.snapshots))
	switch {
	case snapshot.Lifecycle != contract.LifecycleReady:
		t.Errorf("Lifecycle = %q, want %q", snapshot.Lifecycle, contract.LifecycleReady)
	case snapshot.Vehicles == nil || snapshot.AwaitingFirstReport == nil:
		t.Error("an empty fleet arrives as null rather than as empty, giving the client a case that says nothing")
	case snapshot.Summary.Total != 0:
		t.Errorf("Summary.Total = %d, want 0", snapshot.Summary.Total)
	case len(snapshot.Coverage) != len(contract.Zones()):
		t.Errorf("coverage describes %d zones, want all %d even with no fleet", len(snapshot.Coverage), len(contract.Zones()))
	}
}

// The tick is what bounds freshness, so the property worth asserting is that an event ingested before
// a tick is in the snapshot that tick publishes. The rest of the budget is spent past the process
// boundary, and is instrumented rather than claimed (ADR-0009 §9.4).
func TestAnEventIngestedBeforeATickIsInThatTicksSnapshot(t *testing.T) {
	projector, publisher := newPublisher(t)
	publisher.MarkReady()
	viewer, _ := publisher.subscribe()

	publisher.Publish()
	if got := snapshotFrom(t, take(t, viewer.snapshots)).Vehicles; len(got) != 0 {
		t.Fatalf("the fleet began with %d vehicles", len(got))
	}

	reportingVehicle(t, projector, contract.StatusFree)
	publisher.Publish()

	if got := snapshotFrom(t, take(t, viewer.snapshots)).Vehicles; len(got) != 1 || got[0].Label != "LV-0001" {
		t.Errorf("vehicles = %+v, want the one ingested before the tick", got)
	}
}

// Publishing continues whether or not anything changed, because that is what makes silence
// diagnostic: the client reads the absence of a snapshot as a lost connection, and needs no separate
// heartbeat message to do it (ADR-0005 §5.2, §5.7).
func TestPublishingIsUnconditional(t *testing.T) {
	projector, publisher := newPublisher(t)
	reportingVehicle(t, projector, contract.StatusFree)
	publisher.MarkReady()
	viewer, _ := publisher.subscribe()

	for tick := range 3 {
		publisher.Publish()
		if got := snapshotFrom(t, take(t, viewer.snapshots)).Summary.Total; got != 1 {
			t.Fatalf("tick %d published a fleet of %d, want 1 with nothing having changed", tick, got)
		}
	}
}

// A newer snapshot replaces a pending one rather than queueing behind it. That is correct rather than
// merely tolerable — an old snapshot is worthless once a newer exists — and it is only available
// because every message is complete in itself (ADR-0005 §5.1, §5.11).
func TestAPendingSnapshotIsReplacedRatherThanQueued(t *testing.T) {
	projector, publisher := newPublisher(t)
	publisher.MarkReady()
	viewer, _ := publisher.subscribe()

	publisher.Publish()
	reportingVehicle(t, projector, contract.StatusFree)
	publisher.Publish()

	if got := snapshotFrom(t, take(t, viewer.snapshots)).Summary.Total; got != 1 {
		t.Errorf("the pending snapshot describes %d vehicles, want the newest state's 1", got)
	}
	nothingPending(t, viewer.snapshots)
}

// A viewer that stops reading must not be allowed to degrade the others, and must not be held open for
// ever either. While it is merely slow, replacing its pending snapshot is enough; past that it is
// dropped, because blocking the broadcaster on it would penalise every other viewer (ADR-0005 §5.11).
func TestAViewerThatStopsReadingIsDropped(t *testing.T) {
	projector, publisher := newPublisher(t)
	reportingVehicle(t, projector, contract.StatusFree)
	publisher.MarkReady()

	stopped, _ := publisher.subscribe()
	for range maxStalledTicks + 2 {
		publisher.Publish()
	}

	select {
	case <-stopped.done:
	default:
		t.Error("a viewer that never read anything is still connected")
	}

	other, _ := publisher.subscribe()
	publisher.Publish()
	take(t, other.snapshots)
}

// Closing the publisher has to release the open streams, or the process cannot shut down: an SSE
// handler is otherwise blocked on channels nothing will ever write to again.
func TestShuttingDownReleasesOpenStreams(t *testing.T) {
	_, publisher := newPublisher(t)

	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		publisher.Run(ctx)
		close(stopped)
	}()

	viewer, _ := publisher.subscribe()
	stop()
	<-stopped

	select {
	case <-viewer.done:
	default:
		t.Error("a viewer was left connected after the publisher stopped")
	}
}
