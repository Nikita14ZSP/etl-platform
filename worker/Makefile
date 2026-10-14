GO    ?= go
BIN   := bin
IMAGE ?= etl-worker:dev

.PHONY: build test lint run docker clean

build:
	$(GO) build -o $(BIN)/worker ./cmd/worker

test:
	$(GO) test -race ./...

lint:
	golangci-lint run ./...

run: build
	./$(BIN)/worker

# Контекст сборки — корень репозитория, потому что go.mod лежит там.
docker:
	docker build -f Dockerfile -t $(IMAGE) ..

clean:
	rm -rf $(BIN)
