.PHONY: build run clean docker

build:
	go build -o albedo ./cmd/albedo

run: build
	./albedo

clean:
	rm -f albedo
	rm -rf data logs

docker:
	docker-compose up --build -d

deps:
	go mod download
