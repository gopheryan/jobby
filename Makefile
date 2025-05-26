test:
	@go test -v ./...

# requires c compiler
test-race: echo
	CGO_ENABLED=1 go test -race ./...

.PHONY: echo
echo: testdata/testprograms/echo

testdata/testprograms/echo: cmd/echo/main.go
	go build -o testdata/testprograms/echo cmd/echo/main.go
