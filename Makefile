# TunnelBypass — dev tasks. On Windows, use Git Bash / WSL, or run the `go` lines by hand.

BINARY_NAME := tunnelbypass
CMD_PKG := ./cmd

.PHONY: build build-windows run test clean vet staticcheck

build:
	go build -o $(BINARY_NAME) $(CMD_PKG)

# Cross-compile Windows amd64 from Linux/macOS/Git-Bash:
build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME).exe $(CMD_PKG)

run:
	go run $(CMD_PKG)

test:
	go test ./...

clean:
	go clean -cache -testcache
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...
