.PHONY: build install clean test run

build:
	go build -o claude-resume

install: build
	go install

clean:
	rm -f claude-resume

test:
	go test ./...

run: build
	./claude-resume