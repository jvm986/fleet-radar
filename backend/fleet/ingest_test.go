package fleet

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"fleetradar/contract"
)

// The barrier is what makes "registration replay has completed" a definite fact. If it returned
// early the backend would declare itself Ready with the roster still in the queue, and the operator
// would be shown a partial fleet as though it were the whole one — which is the failure F6 and F9
// exist to prevent, and it would be intermittent (ADR-0004 §4.11).
func TestABarrierWaitsForEverythingEnqueuedBeforeIt(t *testing.T) {
	store := NewMemoryStore()
	projector, _ := newProjector(t, store)
	ingest := NewIngest(projector)

	const fleetSize = 200
	for i := range fleetSize {
		ingest.Consume(registration(t, vehicleID(i), "LV-"+vehicleID(i)))
	}
	barrier := ingest.Barrier()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go ingest.Run(ctx)

	<-barrier
	if got := len(store.Snapshot()); got != fleetSize {
		t.Errorf("the barrier was reached with %d of %d vehicles projected", got, fleetSize)
	}
}

func TestIngestProjectsWhatItIsGiven(t *testing.T) {
	store := NewMemoryStore()
	ingest := NewIngest(NewProjector(store, slog.New(slog.DiscardHandler), func() time.Time { return at(3600) }))

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go ingest.Run(ctx)

	ingest.Consume(registration(t, vehicleOne, "LV-0001"))
	ingest.Consume(position(t, vehicleOne, 1, at(1), -115.17, 36.11, 90))
	<-ingest.Barrier()

	if vehicle := store.Snapshot()[0]; vehicle.Position != (contract.Point{-115.17, 36.11}) {
		t.Errorf("Position = %v, want the reported one", vehicle.Position)
	}
}

func vehicleID(i int) string {
	return fmt.Sprintf("b6a1d0c4-2f77-4a1e-bb45-%012d", i)
}
