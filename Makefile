.PHONY: build test lint clean

build:
	go build -tags fts5 -o tasc .

test:
	go test -tags fts5 ./...

lint:
	go vet ./...

clean:
	rm -f tasc
