.PHONY: build test vet tidy fmt install check

# install builds the provider into the local Terraform plugin directory so
# `terraform init` against a local .tf can pick it up via a dev_overrides
# entry in ~/.terraformrc. Useful for end-to-end testing before the first
# Registry-published version exists.
install:
	go build -o ~/.terraform.d/plugins/registry.terraform.io/microwave-sh/microwave/0.0.0/$(shell go env GOOS)_$(shell go env GOARCH)/terraform-provider-microwave .

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

check: vet test build
