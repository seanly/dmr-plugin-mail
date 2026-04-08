.PHONY: build demo-build imap-demo-build test vet tidy clean install install-policy cross-build

BINARY := dmr-plugin-mail
INSTALL_DIR := $(HOME)/.dmr/plugins
POLICY_DIR := $(HOME)/.dmr/etc/policies

build: tidy
	go build -o $(BINARY) .

# Standalone SMTP probe (TLS + AUTH only by default); see cmd/mail-smtp-demo/main.go
demo-build: tidy
	go build -o mail-smtp-demo ./cmd/mail-smtp-demo/

# Standalone IMAP probe (TLS + LOGIN); see cmd/mail-imap-demo/main.go
imap-demo-build: tidy
	go build -o mail-imap-demo ./cmd/mail-imap-demo/

test:
	go test ./... -count=1

vet:
	go vet ./...

tidy:
	go mod tidy

cross-build: tidy
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY)-darwin-arm64 .

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/

install-policy:
	mkdir -p $(POLICY_DIR)
	cp policies/mail.rego $(POLICY_DIR)/

clean:
	rm -f $(BINARY) mail-smtp-demo mail-imap-demo
	rm -f $(BINARY)-linux-amd64 $(BINARY)-linux-arm64
	rm -f $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64
