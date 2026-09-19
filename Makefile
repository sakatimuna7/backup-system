build:
	go build -o backup-system .

test:
	go test ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

verify-systemd:
	systemd-analyze verify contrib/backup-system.timer

fmt:
	gofmt -w *.go
