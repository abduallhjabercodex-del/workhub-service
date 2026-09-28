# Service Level Objectives (SLO), Indicators (SLI), and Error Budgets

This document outlines the reliability targets for the **WorkHub Service**. By establishing clear SLIs and SLOs, we align engineering and product on what "healthy" means for our users, and use Error Budgets to balance feature velocity with system reliability.

---

## 1. Service Level Indicators (SLIs)

SLIs are the actual quantitative measures of the service's behavior. We focus on the two most critical RED metrics for our API layer:

### Availability (Success Rate)
- **Definition:** The proportion of valid HTTP requests that are served without internal errors.
- **Metric:** `sum(rate(workhub_http_requests_total{status!~"5.."}[5m])) / sum(rate(workhub_http_requests_total[5m]))`
- **Exclusions:** 4xx errors (Client errors, Rate Limits, Load Shedding) are considered "successful" from a platform reliability perspective since the system behaved correctly.

### Latency (Performance)
- **Definition:** The proportion of valid HTTP requests served within a specific time threshold.
- **Metric:** `sum(rate(workhub_http_request_duration_seconds_bucket{le="0.25"}[5m])) / sum(rate(workhub_http_request_duration_seconds_count[5m]))`
- **Threshold:** 250ms for the 95th percentile (P95).

---

## 2. Service Level Objectives (SLOs)

SLOs are our target values for the SLIs, measured over a rolling 30-day window.

| Capability   | SLI (Metric)           | SLO (Target) | 30-Day Allowed Failure  |
| :----------- | :--------------------- | :----------- | :---------------------- |
| **Core API** | Availability           | **99.9%**    | ~43 minutes of downtime |
| **Core API** | Latency (P95 < 250ms)  | **99.0%**    | 1% of total requests    |
| **Workers**  | Job Processing Success | **99.9%**    | 0.1% of background jobs |

---

## 3. Error Budgets & Policies

The Error Budget is `100% - SLO`. For a 99.9% Availability SLO, our error budget is **0.1%** of requests in a 30-day window.

### Error Budget Policy

How we behave when the budget is consumed:

1. **Budget > 0% (Healthy):** 
   - Proceed with normal feature development and scheduled deployments.
2. **Budget < 25% Remaining (Warning):**
   - Alert engineering team in Slack (`#workhub-alerts`).
   - Prioritize bug fixes in the current sprint.
3. **Budget Exhausted / < 0% (Critical):**
   - **Feature Freeze:** Halt all non-critical feature deployments.
   - **Reliability First:** 100% of engineering effort shifts to reliability improvements, technical debt reduction, and bug fixes.
   - Requires VP of Engineering approval to bypass the freeze for critical hotfixes.

### Tracking the Budget
Error budgets are monitored continuously via Prometheus and Grafana. Alerts are configured to trigger on **burn rates** (e.g., if we are burning the budget 10x faster than normal, page the on-call engineer immediately).
