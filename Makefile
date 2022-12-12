.PHONY: build clean tool lint help

all: tool build

build:
	go build -v telemetry/cmd/telemetry

tool:
	gofmt -w .

clean:
	rm -rf telemetry
	go clean -i .

help:
	@echo "make: compile packages and dependencies"
	@echo "make tool: run specified go tool"
	@echo "make clean: remove object files and cached files"
