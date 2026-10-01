.PHONY: build run test clean install uninstall zip completions all

BINARY_NAME=kunji
VERSION=1.2.0
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

all: clean build

build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME) validate --help

test:
	go test -v ./...

clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f kunji_*.zip

install: build
	@echo "Installing to /usr/local/bin..."
	sudo mv $(BINARY_NAME) /usr/local/bin/

uninstall:
	@echo "Uninstalling..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

zip: build
	@echo "Creating release zip kunji_$(VERSION).zip..."
	zip kunji_$(VERSION).zip $(BINARY_NAME)

completions: build
	@echo "Generating shell completions into completions/..."
	@mkdir -p completions
	./$(BINARY_NAME) completion bash > completions/kunji.bash
	./$(BINARY_NAME) completion zsh > completions/_kunji
	./$(BINARY_NAME) completion fish > completions/kunji.fish
	./$(BINARY_NAME) completion powershell > completions/_kunji.ps1
	@echo "Wrote completions/{kunji.bash,_kunji,kunji.fish,_kunji.ps1}"
