all:
	go build -o dist/detour .

test:
	go test ./...

profile:
	curl -o server.pprof http://localhost:3811/debug/pprof/profile?seconds=30
	go tool pprof -http : server.pprof