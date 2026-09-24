# Contributing

Thank you for improving SLO Forge.

## Development

Prerequisites: Go 1.27 or later and Docker for the full validation path.

```bash
go mod download
go test -race ./...
go vet ./...
go run ./cmd/slo-forge render -f examples/checkout-api.slo.yaml -o examples/generated
docker run --rm -v "$PWD/examples/generated:/work:ro" \
  --entrypoint /bin/promtool prom/prometheus:v3.14.0 \
  check rules /work/prometheus-rules.yaml
```

Run `gofmt -w` on changed Go files. Generated example changes must be committed so reviewers can inspect policy diffs.

## Pull requests

- Keep changes focused and explain the operational problem.
- Add tests for new behavior and failure cases.
- Update documentation when the schema or generated output changes.
- Avoid breaking the v1alpha1 schema without an upgrade path.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
