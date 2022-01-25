include .env

compile:
	@echo "Building taskmaster"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o dist/taskmaster src/main.go
	@echo "Building test app"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o test/main test/main.go
run:
	./dist/taskmaster --command="./test/main"

purge:
	rm -v ./dist/taskmaster
	rm -v ./test/main
test:
	pwd
	compile
	run
	purge


docker-build:
	docker build . -f .docker/Dockerfile --tag $(DOCKER_SERVER_HOST)/$(DOCKER_PROJECT_PATH):$(DOCKER_IMAGE_VERSION)

docker-push:
	docker push $(DOCKER_SERVER_HOST)/$(DOCKER_PROJECT_PATH):$(DOCKER_IMAGE_VERSION)