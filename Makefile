.PHONY: check complexity fmt lint

check:
	go test -buildvcs=false ./...
	$(MAKE) complexity

complexity:
	go run -buildvcs=false github.com/fzipp/gocyclo/cmd/gocyclo -over 10 -ignore '_test.go' .

fmt:
	go fmt ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet -buildvcs=false ./...
	$(MAKE) complexity
