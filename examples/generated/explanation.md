# checkout-api / availability

The proportion of checkout requests that do not return a server error.

- Objective: **99.9%** over a rolling **30-day** window
- Error budget: **0.1%** (approximately **43m 12s** of bad time per 30 days)
- Ownership: `team=checkout`, `tier=1`
- Runbook: https://github.com/kyan9400/slo-forge/blob/main/docs/runbooks/high-burn-rate.md

## Alert policy

| Route | Long window | Short window | Burn rate | Hold |
|---|---:|---:|---:|---:|
| page | 1h | 5m | 14.4x | 2m |
| page | 6h | 30m | 6.0x | 15m |
| ticket | 1d | 2h | 3.0x | 1h |
| ticket | 3d | 6h | 1.0x | 3h |

Both the long and short windows must breach before an alert fires. This keeps detection fast while filtering brief spikes.

## Generated artifacts

- `prometheus-rules.yaml`: native recording and alerting rules.
- `prometheusrule.yaml`: the same rules packaged for Prometheus Operator.
- `grafana-dashboard.json`: an importable overview of availability and burn rate.
- `explanation.md`: this reviewable policy summary.
