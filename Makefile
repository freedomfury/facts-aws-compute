.PHONY: build clean

BINARY := dist/facts-aws-compute
CMD    := ./cmd/facts-aws-compute

build:
	mkdir -p dist && \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)
