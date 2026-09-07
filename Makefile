# The whole toolchain is the Go toolchain, Node LTS and make — nothing else to install
# (ADR-0001 §1.6). staticcheck is pinned as a tool dependency in backend/go.mod rather than
# installed globally.
#
# This grows as the system does: `make dev` and `make generate` arrive with the read path
# and the web client.

.PHONY: test check

test:
	cd backend && go test ./...

check:
	test -z "$$(cd backend && gofmt -l .)" || { cd backend && gofmt -l . && exit 1; }
	cd backend && go vet ./...
	cd backend && go tool staticcheck ./...
