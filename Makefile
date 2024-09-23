all: build

BUILD=.tjob
proto:
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/service.proto

certs:
	@openssl req -x509 -newkey Ed25519 -days 365 -nodes -keyout $(BUILD)/ca.key -out $(BUILD)/ca.crt -subj "/CN=issuer"
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/svc.key -out $(BUILD)/svc.crt -subj "/CN=tjobs" -addext "subjectAltName=DNS:localhost" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/cli.key -out $(BUILD)/cli.crt -subj "/CN=alice" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/bob.key -out $(BUILD)/bob.crt -subj "/CN=bob" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30

build: 
	@mkdir -p $(BUILD)
	GOBIN=$(PWD)/$(BUILD) go install ./cmd/...
	@touch $(BUILD)

examples:
	@mkdir -p $(BUILD)
	GOBIN=$(PWD)/$(BUILD) go install ./examples/...

install:
	go install ./cmd/...

clean:
	rm -r $(BUILD)
