package api

import (
	"log/slog"
	"net/http"
)

// StreamPath is the one endpoint the client calls. There is no other: the client requests nothing
// beyond the GET that opens the stream, and there is no initial-state endpoint because the opening
// frames are the initial state (ADR-0005 §5.4, §5.8).
const StreamPath = "/api/stream"

// reconnectHint tells the browser how long to wait before reopening a dropped stream. Reconnection is
// the transport's own, which is one of the reasons for choosing it — but three seconds of the default
// is a long time to look at a view marked untrustworthy.
const reconnectHint = "retry: 1000\n\n"

// Stream carries the fleet to one viewer over Server-Sent Events. SSE matches the shape of the
// problem exactly: the product is observe-only, so a server-to-client channel is all there is to
// model, and in Go it is writing to a ResponseWriter and flushing — no upgrade, no library, and
// browser-native reconnection. A bidirectional transport would be capability the product deliberately
// excludes (ADR-0005 §5.5, §5.6).
func (p *Publisher) Stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	control := http.NewResponseController(w)
	send := func(frames ...[]byte) bool {
		for _, rendered := range frames {
			if _, err := w.Write(rendered); err != nil {
				return false
			}
		}
		return control.Flush() == nil
	}

	viewer, opening := p.subscribe()
	defer p.unsubscribe(viewer)

	if !send(append([][]byte{[]byte(reconnectHint)}, opening...)...) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return

		case <-viewer.done:
			// Either the process is shutting down, or this viewer stopped reading for long enough that
			// holding the connection open was telling nobody anything.
			p.log.Warn("dropped a viewer that was not keeping up", slog.String("client", r.RemoteAddr))
			return

		case rendered := <-viewer.routes:
			if !send(rendered) {
				return
			}

		case rendered := <-viewer.snapshots:
			// Geometry goes first when both are pending, so a snapshot never references a route the
			// viewer has not been given. A select alone would choose between them at random.
			select {
			case geometry := <-viewer.routes:
				if !send(geometry) {
					return
				}
			default:
			}
			if !send(rendered) {
				return
			}
		}
	}
}
