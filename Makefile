# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## confirm: make sure user wants to proceed with operation
.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## vendor: tidy and vendor dependencies
.PHONY: vendor
vendor:
	@echo 'Tidying and verifying module dependencies...'
	go mod tidy
	go mod verify
	@echo 'Vendoring dependencies...'
	go mod vendor

## audit: format, vet and test all code
.PHONY: audit
audit: vendor
	@echo 'Formatting code...'
	go fmt ./...
	@echo 'Vetting code...'
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	@echo 'Running tests...'
	go test -race -vet=off ./...

## test_coverage: run tests and check test coverage
.PHONY: test_coverage
test_coverage:
	go test -coverprofile=/tmp/profile.out ./...
	go tool cover -html=/tmp/profile.out

# ==================================================================================== #
# BUILD
# ==================================================================================== #

## build: build the cmd/api application
.PHONY: build
build:
	@echo 'Building cmd/api...
	go build -ldflags='-s' -o=./bin/api ./cmd/api
	GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/api ./cmd/api

## run: run the cmd/api application
.PHONY: run
run: audit build
	./bin/api

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## tls: generate self-signed certificate (TLS certificate)
.PHONY: tls
tls:
	mkdir tls
	cd tls
	go run "$(go env GOROOT)/src/crypto/tls/generate_cert.go" --rsa-bits=2048 --host=localhost

# cert: generate private and public RSA keys
.PHONY: cert
cert:
	mkdir assets/cert
	openssl genrsa -out cert/id_rsa 4096
	openssl rsa -in cert/id_rsa -pubout -out cert/id_rsa.pub