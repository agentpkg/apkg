.PHONY: build
build:
	go build -o apkg ./cmd/apkg

.PHONY: test
test:
	go test -count=1 -race ./...
