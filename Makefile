build:
	@go build -o bin/webserver

run: build
	@./bin/webserver

test:
	@go test -v ./...

clean:
	@rm -rf bin