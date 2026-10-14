GO    ?= go
BIN   := bin
IMAGE ?= etl-worker:dev

.PHONY: build test lint run enqueue docker clean

build:
	$(GO) build -o $(BIN)/worker ./cmd/worker
	$(GO) build -o $(BIN)/enqueue ./cmd/enqueue

test:
	$(GO) test -race ./...

lint:
	golangci-lint run ./...

run: build
	./$(BIN)/worker

# make enqueue CMD='echo hello'
enqueue: build
	./$(BIN)/enqueue -cmd "$(or $(CMD),echo hello)"

# Контекст сборки — корень репозитория, потому что go.mod лежит там.
docker:
	docker build -f Dockerfile -t $(IMAGE) ..

clean:
	rm -rf $(BIN)
