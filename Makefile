.PHONY: build run down clean test

build:
	docker compose build

test:
	docker run --rm -v "$(PWD)/server":/src -w /src golang:1.25-alpine sh -c "go mod tidy && go test ./..."

run:
	docker compose up -d --build

down:
	docker compose down

clean:
	docker compose down -v --rmi local --remove-orphans
	rm -rf data
