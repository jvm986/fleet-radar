# The whole toolchain is the Go toolchain, Node LTS and make — nothing else to install
# (ADR-0001 §1.6). staticcheck is pinned as a tool dependency in backend/go.mod rather than
# installed globally.
#
# This grows as the system does: `make dev` and `make generate` arrive with the read path
# and the web client.

.PHONY: dev test check

# The web client joins this once it exists; for now it is the backend alone, serving the stream on
# :8080.
dev:
	cd backend && go run ./cmd/fleetradar

test:
	cd backend && go test ./...

check:
	test -z "$$(cd backend && gofmt -l .)" || { cd backend && gofmt -l . && exit 1; }
	cd backend && go vet ./...
	cd backend && go tool staticcheck ./...
