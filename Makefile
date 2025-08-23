# Makefile for fs-xfer project

PROTO_DIR=protos
OUT_DIR=pkg/generated
PROTOC=protoc
PROTOC_GEN_GO=protoc-gen-go

.PHONY: all
all: protos

.PHONY: protos
protos:
	@mkdir -p $(OUT_DIR)
	$(PROTOC) \
		--go_out=$(OUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		-I $(PROTO_DIR) \
		$(PROTO_DIR)/**/*.proto

.PHONY: clean
clean:
	rm -rf $(OUT_DIR)/*

.PHONY: run
run:
	docker-compose up --build

.PHONY: client
client:
	go install ./cmd/fs
