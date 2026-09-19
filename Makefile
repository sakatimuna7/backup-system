build:
	go build -o backup-system .

test:
	go test ./...

fmt:
	gofmt -w *.go
