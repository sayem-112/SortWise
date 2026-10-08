.PHONY: install test build dev clean

install:
	npm install
	go mod download

test:
	go test ./cmd/... ./internal/... ./migrations/...
	npm run test:ci

build:
	npm run build
	go build -trimpath -ldflags "-s -w -H=windowsgui" -o sortwise.exe ./cmd/sortwise

dev:
	go run ./cmd/sortwise --no-open --no-tray

clean:
	go clean
