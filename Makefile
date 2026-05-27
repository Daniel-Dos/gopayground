.PHONY: test lint build build-ui build-producer run run-ui run-producer \
        run-producer-serve docker-up docker-down clean copy-docs

test:
	go test ./... -race -count=1 -timeout=120s

lint:
	golangci-lint run ./...

build:
	go build -o bin/consumer ./cmd/consumer

copy-docs:
	@mkdir -p internal/ui/static/docs
	@cp -r docs/* internal/ui/static/docs/ 2>/dev/null || true

build-ui: copy-docs
	go build -o bin/ui ./cmd/ui

build-producer:
	go build -o bin/producer ./cmd/producer

run:
	go run ./cmd/consumer

run-ui: copy-docs
	go run ./cmd/ui

run-producer:
	go run ./cmd/producer

run-producer-serve:
	go run ./cmd/producer serve --port 8082

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -rf bin/ internal/ui/static/docs/
