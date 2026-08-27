.PHONY: build check fmt generate test validate

build:
	go build -trimpath -o bin/slo-forge ./cmd/slo-forge

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

generate:
	go run ./cmd/slo-forge render -f examples/checkout-api.slo.yaml -o examples/generated

test:
	go test -race ./...

validate: generate
	docker run --rm -v "$$PWD/examples/generated:/work:ro" --entrypoint /bin/promtool prom/prometheus:v3.14.0 check rules /work/prometheus-rules.yaml

check: test generate validate
	go vet ./...
