BIN_DIR := $(CURDIR)/bin
GOCACHE := $(CURDIR)/.cache

.PHONY: build-server build-client build-monitor build-all clean

build-server:
	@mkdir -p $(BIN_DIR)
	GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/server ./services/server

build-client:
	@mkdir -p $(BIN_DIR)
	GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/client ./services/client

build-monitor:
	@mkdir -p $(BIN_DIR)
	GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/monitor ./services/monitor

build-all: build-server build-client build-monitor

clean:
	rm -rf $(BIN_DIR)
