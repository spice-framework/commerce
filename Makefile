.PHONY: bootstrap check compatibility fmt lint security smoke test verify

bootstrap:
	go mod tidy -diff
	go -C tools mod tidy -diff
	go mod download
	go -C tools mod download
	go mod vendor

check:
	go run ./internal/qualitygate -mode=check

compatibility:
	go run -mod=readonly ./internal/corecompat -line=all

fmt:
	go run ./internal/qualitygate -mode=fmt

lint:
	go run ./internal/qualitygate -mode=lint

security:
	go run ./internal/qualitygate -mode=security

smoke:
	go run ./internal/qualitygate -mode=smoke

test:
	go run ./internal/qualitygate -mode=test

verify:
	go run ./internal/qualitygate -mode=verify
