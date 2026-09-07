// Package api is the read path. One stream per viewer, a complete snapshot on every tick, and
// nothing travelling the other way: the system is observe-only, so a server-to-client channel is the
// whole of what needs modelling, and that single fact settles more of this than any performance
// consideration (ADR-0005).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"fleetradar/contract"
	"fleetradar/derive"
	"fleetradar/fleet"
)

// maxStalledTicks is how many consecutive publications a viewer may fail to take before it is
// disconnected. Five seconds of not reading is a viewer that will not catch up, and a slow one must
// not be allowed to degrade the others — blocking the broadcaster on it would do exactly that
// (ADR-0005 §5.11).
const maxStalledTicks = 25

// Publisher owns the tick. It holds a read-only view of the store, which is where "events are the
// only writer of state" stops being a convention: the write methods are not reachable from here
// (ADR-0004 §4.3).
type Publisher struct {
	store fleet.Reader
	zones []contract.Zone
	now   func() time.Time
	log   *slog.Logger

	// config never changes, so it is rendered once.
	config []byte

	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	lifecycle   contract.Lifecycle
	// routes is the last published route frame, kept so a viewer joining mid-journey is given the
	// geometry currently on the map rather than waiting for the next change (ADR-0005 §5.5).
	routes   []byte
	routeIDs []string
}

func NewPublisher(store fleet.Reader, log *slog.Logger, now func() time.Time) *Publisher {
	config, err := frame(contract.MessageConfig, contract.NewConfig())
	if err != nil {
		panic(fmt.Sprintf("api: the config will not serialise: %v", err))
	}

	return &Publisher{
		store:       store,
		zones:       contract.Zones(),
		now:         now,
		log:         log,
		config:      config,
		subscribers: make(map[*subscriber]struct{}),
		lifecycle:   contract.LifecycleStarting,
	}
}

// MarkReady says that registration replay has completed, so the fleet on screen is the whole fleet.
// Until then the snapshot says the view is still filling, because showing a partial fleet as though
// it were complete is the failure F6 exists to prevent (ADR-0004 §4.11).
func (p *Publisher) MarkReady() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lifecycle = contract.LifecycleReady
}

// Run publishes until the context ends, then disconnects every viewer so their handlers return.
func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(contract.TickInterval)
	defer ticker.Stop()
	defer p.closeAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Publish()
		}
	}
}

// Publish derives one generation of state and sends it to every viewer. It is exported so tests can
// drive the read path without waiting on a ticker.
//
// It publishes unconditionally, whether or not anything changed. That is not laziness: because a
// snapshot always arrives, its absence is unambiguous, and the client's watchdog needs no separate
// heartbeat message to tell "nothing is happening" from "the connection is dead" (ADR-0005 §5.2,
// §5.7).
func (p *Publisher) Publish() {
	vehicles := p.store.Snapshot()

	p.mu.Lock()
	lifecycle := p.lifecycle
	p.mu.Unlock()

	snapshot, err := frame(contract.MessageSnapshot, derive.Snapshot(vehicles, p.zones, lifecycle, p.now()))
	if err != nil {
		// Nothing a viewer can be told, and the next tick is 200 ms away.
		p.log.Error("could not serialise the snapshot", slog.String("error", err.Error()))
		return
	}

	routes := p.routesIfChanged(derive.Routes(vehicles))

	p.mu.Lock()
	defer p.mu.Unlock()
	for viewer := range p.subscribers {
		if routes != nil {
			viewer.offer(viewer.routes, routes)
		}
		if !viewer.offer(viewer.snapshots, snapshot) {
			viewer.stalled++
			if viewer.stalled > maxStalledTicks {
				p.disconnect(viewer)
			}
			continue
		}
		viewer.stalled = 0
	}
}

// routesIfChanged renders the route set only when it differs from the one already sent, and reports
// nil when it does not. Comparing ids is enough: a revised journey is assigned a new route id, so an
// id that is still present still means the same path (ADR-0003 §3.8).
func (p *Publisher) routesIfChanged(routes contract.Routes) []byte {
	ids := make([]string, len(routes.Routes))
	for i, route := range routes.Routes {
		ids[i] = route.RouteID
	}

	// routeIDs is written only here, on the tick goroutine, so comparing it needs no lock; the
	// rendered frame does, because a viewer joining reads it.
	if slices.Equal(ids, p.routeIDs) {
		return nil
	}

	rendered, err := frame(contract.MessageRoutes, routes)
	if err != nil {
		p.log.Error("could not serialise the routes", slog.String("error", err.Error()))
		return nil
	}

	p.mu.Lock()
	p.routeIDs, p.routes = ids, rendered
	p.mu.Unlock()
	return rendered
}

// subscriber is one viewer. It holds at most one pending frame of each kind, and a newer frame
// replaces a pending one rather than queueing behind it — which is correct rather than merely
// tolerable, because both messages are complete in themselves and an old one is worthless once a
// newer exists. This is only available because messages are snapshots; with deltas nothing could be
// dropped (ADR-0005 §5.1, §5.11).
type subscriber struct {
	snapshots chan []byte
	routes    chan []byte
	done      chan struct{}
	stalled   int
}

// offer places a frame if the slot is free, or replaces what is pending. It reports whether the
// viewer had taken the previous one.
func (s *subscriber) offer(slot chan []byte, rendered []byte) bool {
	select {
	case slot <- rendered:
		return true
	default:
	}

	// What is pending is stale, so it makes way. Draining leaves the slot empty — nothing else sends
	// to it — so the replacement cannot block.
	select {
	case <-slot:
	default:
	}
	slot <- rendered
	return false
}

// subscribe registers a viewer and returns the frames it is owed before the stream proper: the
// config it renders its legend from, and the geometry of the routes currently on the map. Config
// goes first so that it is present before any snapshot needs it (ADR-0005 §5.8).
func (p *Publisher) subscribe() (*subscriber, [][]byte) {
	viewer := &subscriber{
		snapshots: make(chan []byte, 1),
		routes:    make(chan []byte, 1),
		done:      make(chan struct{}),
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.subscribers[viewer] = struct{}{}
	opening := [][]byte{p.config}
	if p.routes != nil {
		opening = append(opening, p.routes)
	}
	return viewer, opening
}

func (p *Publisher) unsubscribe(viewer *subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subscribers, viewer)
}

// disconnect must be called holding the lock.
func (p *Publisher) disconnect(viewer *subscriber) {
	delete(p.subscribers, viewer)
	close(viewer.done)
}

func (p *Publisher) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for viewer := range p.subscribers {
		p.disconnect(viewer)
	}
}

// frame renders one message as a Server-Sent Event. The event name is the message kind, and the
// payload is a single line, which it always is because encoding/json never emits a raw newline.
func frame(event contract.MessageKind, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	rendered := make([]byte, 0, len(body)+len(event)+16)
	rendered = append(rendered, "event: "...)
	rendered = append(rendered, event...)
	rendered = append(rendered, "\ndata: "...)
	rendered = append(rendered, body...)
	return append(rendered, "\n\n"...), nil
}
