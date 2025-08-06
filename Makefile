.PHONY: build install clean test run

build:
	go build -o claude-resume ./cmd/claude-resume

install:
	go install ./cmd/claude-resume

clean:
	rm -f claude-resume

test:
	go test ./...

run: build
	./claude-resume