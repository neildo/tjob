all: build

BUILD=.tjob
proto:
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/service.proto

$(BUILD):
	@mkdir -p $(BUILD)

examples: $(BUILD)
	GOBIN=$(PWD)/$(BUILD) go install ./examples/...

certs: $(BUILD)
	@echo generate self-signed certs for CA, API, and CLIs under $(BUILD)/
	@openssl req -x509 -newkey Ed25519 -days 365 -nodes -keyout $(BUILD)/ca.key -out $(BUILD)/ca.crt -subj "/CN=issuer"
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/svc.key -out $(BUILD)/svc.crt -subj "/CN=tjobs" -addext "subjectAltName=DNS:localhost" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/cli.key -out $(BUILD)/cli.crt -subj "/CN=alice" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30
	@openssl req -x509 -newkey Ed25519 -nodes -keyout $(BUILD)/other.key -out $(BUILD)/other.crt -subj "/CN=other" -CA $(BUILD)/ca.crt -CAkey $(BUILD)/ca.key -days 30

build: $(BUILD) certs
	@echo build API and CLI
	@GOBIN=$(PWD)/$(BUILD) go install ./cmd/...
	@ls -alh $(BUILD)

test:
	@go test -v ./... -count=1

lint:
	golangci-lint run \
		--disable wsl \
		--disable nlreturn \
		--disable gci \
		--disable exportloopref \
		--disable godot \
		--disable forbidigo \
		--disable cyclop \
		--disable depguard \
		--disable execinquery \
		--disable exhaustruct \
		--disable funlen \
		--disable gocognit \
		--disable godox \
		--disable gomnd \
		--disable gosec \
		--disable ireturn \
		--disable lll \
		--disable mnd \
		--disable stylecheck \
		--enable-all \
		./...

install: 
	go install ./cmd/...

clean:
	@rm -rf $(BUILD)
