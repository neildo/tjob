all: build

BUILD=.tjob
proto:
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/service.proto

certs:
	openssl req -x509 -newkey Ed25519 -days 365 -nodes -keyout $(BUILD)/ca-key.pem -out $(BUILD)/ca-cert.pem -subj "/CN=issuer"
	openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/svc-key.pem -out $(BUILD)/svc-cert.pem -subj "/CN=tjobs" -addext "subjectAltName=DNS:localhost" -CA $(BUILD)/ca-cert.pem -CAkey $(BUILD)/ca-key.pem -days 30
	openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/cli-key.pem -out $(BUILD)/cli-cert.pem -subj "/CN=alice" -CA $(BUILD)/ca-cert.pem -CAkey $(BUILD)/ca-key.pem -days 30
	openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/bob-key.pem -out $(BUILD)/bob-cert.pem -subj "/CN=bob" -CA $(BUILD)/ca-cert.pem -CAkey $(BUILD)/ca-key.pem -days 30

build:
	@mkdir -p $(BUILD)
	GOBIN=$(PWD)/$(BUILD) go install ./cmd/...
	@touch $(BUILD)

examples:
	@mkdir -p $(BUILD)
	GOBIN=$(PWD)/$(BUILD) go install ./examples/...

install: build
	go install ./cmd/...

clean:
	rm -r $(BUILD)

vm-reset:
	sudo /Applications/VMware\ Fusion.app/Contents/Library/vmnet-cli --stop
	sudo /Applications/VMware\ Fusion.app/Contents/Library/vmnet-cli --start
