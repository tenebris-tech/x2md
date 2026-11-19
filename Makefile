# ******************************************************************************
# * Copyright (c) 2026 Tenebris Technologies Inc.                              *
# * Please see LICENSE file for details.                                       *
# ******************************************************************************
#
# x2md Makefile
#
#   make              run the full test suite, then build if it passed
#   make test         run the full test suite (the one gate, for developers and CI)
#   make build        build the binaries, no tests
#   make clean        remove build output and drop the test cache
#   make fmt          rewrite Go files with gofmt (make test only verifies)
#   make test-private run the tests that need the private/ document corpus

BIN_DIR := bin
BINARY  := $(BIN_DIR)/x2md

.PHONY: all test build clean fmt test-private

all: test
	@$(MAKE) --no-print-directory build

test:
	@./test.sh

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cli

clean:
	rm -rf $(BIN_DIR)
	go clean -testcache

fmt:
	gofmt -w .

# Not part of "make test": the corpus in private/ is not in the repository.
# Fails, naming the file, if any document it needs is missing.
test-private:
	go test -race -count=1 -tags private .
