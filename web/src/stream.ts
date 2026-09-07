import type { Config, MessageKind, Routes, Snapshot } from "./contract.generated";

/**
 * The stream is the whole of the client's conversation with the backend: one GET, and then everything
 * arrives. There is no other request, no polling, and no way to send anything back — the product is
 * observe-only, so a server-to-client channel is exactly the shape of the problem (ADR-0005 §5.4).
 *
 * EventSource is a browser primitive, so this needs no dependency, and reconnection is the transport's
 * own. On reconnect the next snapshot replaces client state wholesale, which is why there is no resync
 * protocol here: every message is complete in itself (ADR-0005 §5.8, §5.10).
 */
export interface StreamHandlers {
  onConfig: (config: Config) => void;
  onRoutes: (routes: Routes) => void;
  onSnapshot: (snapshot: Snapshot) => void;
  /** Called when the transport itself reports trouble. It is a supplementary signal only: a stalled
   * connection can stay open and silent, so what is authoritative is the client's own watchdog
   * noticing that snapshots have stopped arriving (ADR-0005 §5.9). */
  onTransportError: () => void;
}

export const streamPath = "/api/stream";

/** open connects and returns the function that closes it again. */
export function open(handlers: StreamHandlers, path = streamPath): () => void {
  const source = new EventSource(path);

  // The event names are the message kinds from the contract, so renaming one in Go stops this
  // compiling rather than silently delivering nothing.
  listen(source, "config", handlers.onConfig);
  listen(source, "routes", handlers.onRoutes);
  listen(source, "snapshot", handlers.onSnapshot);
  source.addEventListener("error", handlers.onTransportError);

  return () => source.close();
}

function listen<T>(source: EventSource, kind: MessageKind, handle: (message: T) => void): void {
  source.addEventListener(kind, (event: MessageEvent<string>) => {
    handle(JSON.parse(event.data) as T);
  });
}
