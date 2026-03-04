include .env

.PHONY: build test lint clean run docker-build docker-push

build:
	@echo "Building taskmaster"
	@go build -o dist/taskmaster ./cmd/taskmaster
	@echo "Building testapp"
	@go build -o dist/testapp ./test/testapp

test:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -rf dist/

run: build
	./dist/taskmaster ./dist/testapp

docker-build:
	docker build . -f .docker/Dockerfile --tag $(DOCKER_SERVER_HOST)/$(DOCKER_PROJECT_PATH):$(DOCKER_IMAGE_VERSION)

docker-push:
	docker push $(DOCKER_SERVER_HOST)/$(DOCKER_PROJECT_PATH):$(DOCKER_IMAGE_VERSION)
