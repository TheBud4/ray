.PHONY: build install test vet fmt fmt-check smoke ci

build:
	go build ./...

install:
	go install .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to be run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Roda o binário de verdade num RAY_HOME descartável (não instala nada).
smoke:
	bash scripts/smoke.sh

ci: fmt-check vet test smoke
