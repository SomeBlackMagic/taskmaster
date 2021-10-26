include .env
compile:
	@echo "Building $(GOFILE) to $(GONAME)"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o $(GONAME) $(GOFILE)
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o ./test/$(GONAME) ./test/$(GOFILE)
run:
	./main --command="./test/main"
purge:
	rm -v ./main
	rm -v ./test/main
tst:
	pwd
	compile
	run
	purge