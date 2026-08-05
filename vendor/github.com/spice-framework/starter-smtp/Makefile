.PHONY: check fmt integration verify

check:
	go run ./internal/qualitygate -mode=check

fmt:
	go run ./internal/qualitygate -mode=fmt

integration:
	go test -tags=integration -count=1 ./integration/...

verify:
	go run ./internal/qualitygate -mode=verify
