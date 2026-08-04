BINARY := diss
INSTALL_DIR ?= $(HOME)/.local/bin
VERSION ?= dev
COMMIT ?= dev
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test install build-all clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

install:
	./install.sh

build-all:
	mkdir -p releases
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-windows-amd64.exe .

clean:
	rm -f $(BINARY) $(BINARY).exe
