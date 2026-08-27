# Runbook: high SLO burn rate

## Signal

One of the `SLOBudgetBurn*` alerts is firing because both its long and short windows exceed the configured burn-rate threshold.

## First five minutes

1. Acknowledge the page and open the linked dashboard.
2. Confirm whether the total-request denominator is present. A missing denominator can indicate telemetry loss rather than a service incident.
3. Compare the bad-event ratio across the 5-minute, 1-hour, and 6-hour windows.
4. Check recent deploys, feature flags, dependency health, and regional traffic shifts.
5. If users are affected, declare an incident and assign an incident commander.

## Mitigation order

1. Roll back or disable the most recent risky change.
2. Shift traffic away from an unhealthy region or dependency when the platform supports it.
3. Shed non-critical work or reduce concurrency to protect the success path.
4. Add capacity only when saturation is the demonstrated cause.

Avoid changing the SLO target, alert threshold, or query during an active incident. Preserve the signal until the service is stable.

## Verification

- The short-window burn rate returns below 1x.
- The error ratio and request volume are both present and credible.
- Synthetic and user-journey checks recover.
- No new alerts appear during the observation period.

## Follow-up

- Record budget consumed and customer impact.
- Link the incident review to the alert or service catalog entry.
- Add a regression test, safer rollout control, or observability improvement.
- Review whether the indicator still represents user-visible reliability.
