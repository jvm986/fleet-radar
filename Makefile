# The whole toolchain is the Go toolchain, Node with pnpm, and make — nothing else to install
# (ADR-0001 §1.6). staticcheck is pinned as a tool dependency in backend/go.mod, and Biome as a
# dev dependency in web/package.json, so neither is a global install.

GENERATED := web/src/contract.generated.ts

.PHONY: dev test check generate

# One command from a fresh clone to a running system. The web client proxies /api to the backend, so
# the browser talks to one origin and the backend serves only the stream.
dev: web/node_modules
	@trap 'kill 0' EXIT INT TERM; \
	(cd backend && go run ./cmd/fleetradar) & \
	(cd web && pnpm dev) & \
	wait

# Go types are the single source of truth for the contract; the TypeScript is generated from them and
# the output is committed (ADR-0001 §1.4).
generate:
	cd backend && go run ./cmd/tsgen > ../$(GENERATED)

test: web/node_modules
	cd backend && go test ./...
	cd web && pnpm test

# The gate, run locally and by CI from the same target. It fails if the generated TypeScript is out of date,
# because Go being the source of truth holds only while drift is visible (ADR-0009 §9.7).
check: web/node_modules
	test -z "$$(cd backend && gofmt -l .)" || { cd backend && gofmt -l . && exit 1; }
	cd backend && go vet ./...
	cd backend && go tool staticcheck ./...
	cd backend && go run ./cmd/tsgen | diff -u ../$(GENERATED) -
	cd web && pnpm check
	cd web && pnpm typecheck

web/node_modules: web/package.json web/pnpm-lock.yaml
	cd web && pnpm install
	@touch web/node_modules
