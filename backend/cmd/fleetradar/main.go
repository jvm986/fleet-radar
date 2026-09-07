// Command fleetradar runs the whole backend in one process: the simulated source behind the consumer
// interface, ingest, the store, and the read path. One process because the boundary that matters is
// the consumer interface — the Kafka client seam — and a second process would have to invent a
// transport between them that models nothing real (ADR-0004 §4.1).
//
// The data flows one way:
//
//	simulator ──JSON bytes──▶ consumer ──▶ validate ──▶ projection ──▶ store
//	                                                                    │
//	                                    ┌───────── 200ms tick ──────────┘
//	                                    ▼
//	                                 derive ──▶ snapshot ──SSE──▶ browser
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fleetradar/api"
	"fleetradar/fleet"
	"fleetradar/sim"
)

const (
	address         = ":8080"
	shutdownTimeout = 2 * time.Second
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := fleet.NewMemoryStore()
	ingest := fleet.NewIngest(fleet.NewProjector(store, log, time.Now))

	// The publisher is given the store as a fleet.Reader, so the write methods are not reachable from
	// the serving layer. "Events are the only writer of state" is a property of this signature rather
	// than a convention that holds until somebody adds a handler (ADR-0004 §4.3).
	publisher := api.NewPublisher(store, log, time.Now)

	go ingest.Run(ctx)
	go publisher.Run(ctx)

	// A simulation that cannot be reproduced cannot be debugged, and the cost of being able to is one
	// log line (ADR-0007 §7.11).
	seed := uint64(time.Now().UnixNano())
	source := sim.New(ingest, log, time.Now, seed)
	log.Info("fleet radar starting", slog.String("address", address), slog.Uint64("seed", seed))

	// Replay the roster, then wait for it to have been projected rather than merely emitted, and only
	// then say the view is complete. Ready is a position in the stream (ADR-0003 §3.9, ADR-0004 §4.11).
	source.Replay()
	<-ingest.Barrier()
	publisher.MarkReady()
	go source.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+api.StreamPath, publisher.Stream)
	server := &http.Server{Addr: address, Handler: mux}

	go func() {
		<-ctx.Done()
		// The publisher drops every viewer when its context ends, so the open streams return and
		// Shutdown is not left waiting on connections that would otherwise never close.
		closing, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(closing); err != nil {
			log.Error("could not shut down cleanly", slog.String("error", err.Error()))
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("could not serve", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("fleet radar stopped")
}
