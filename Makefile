AGENTS_DIR := agents-bin

.PHONY: agents all rsd rsctl

all: rsd rsctl agents

rsd:
	go build -o rsd ./cmd/rsd

rsctl:
	go build -o rsctl ./cmd/rsctl

agents:
	@mkdir -p $(AGENTS_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(AGENTS_DIR)/linux-amd64 ./cmd/rs-agent
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(AGENTS_DIR)/linux-arm64 ./cmd/rs-agent
	GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(AGENTS_DIR)/linux-386 ./cmd/rs-agent
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(AGENTS_DIR)/darwin-amd64 ./cmd/rs-agent
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(AGENTS_DIR)/darwin-arm64 ./cmd/rs-agent
	@echo "agents built in $(AGENTS_DIR)/"
