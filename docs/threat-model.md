# Threat model

## Assets and trust boundaries

SLO Forge handles configuration supplied by a CLI user or HTTP client. It produces text artifacts and does not hold cloud, cluster, Prometheus, or Grafana credentials.

The primary trust boundary is the HTTP request body. Generated content crosses a second boundary when an operator reviews and deploys it through another system.

## Considered threats

| Threat | Control |
|---|---|
| Oversized request exhausts memory | The API caps request bodies at 1 MiB. |
| Slow clients retain connections | Read-header, read, write, and idle timeouts are configured. |
| Unexpected schema fields hide mistakes | YAML decoding rejects unknown fields and multiple documents. |
| Query text causes command execution | The compiler never invokes a shell or evaluates PromQL. |
| Path traversal through API input | The API returns JSON and performs no filesystem writes. |
| Compromised runtime image | The final image is `scratch`, runs as UID/GID 65532, and contains one static binary. |
| Malicious dependency update | Dependencies and GitHub Actions are reviewed through Dependabot; actions are pinned to commit SHAs. |
| Generated policy is deployed without review | The tool does not apply resources; CI and GitOps remain separate controls. |

## Residual risks

- A syntactically valid PromQL expression may still be expensive or semantically wrong for a particular Prometheus deployment.
- Authentication and rate limiting are intentionally delegated to an ingress or API gateway. Do not expose the service directly to an untrusted network.
- Consumers must review generated manifests before deployment and enforce their own admission policies.

## Reporting

Use GitHub private vulnerability reporting. Do not include sensitive deployment details in a public issue.
