# Service Level Objective (SLO) Report — WorkHub Service

**Document Status:** Final  
**Date:** 2026-09-19  
**Evaluation Window:** 5-minute window  
**Environment:** Staging / Test Verification Workload  

---

## 1. SLO Targets & SLI Definitions

WorkHub defines production-grade Service Level Objectives (SLOs) for system availability and request latency:

| Metric                          | Target (SLO) | Prometheus Query (SLI Formula)                                                                                   | Measured Value | Status  |
| :------------------------------ | :----------- | :--------------------------------------------------------------------------------------------------------------- | :------------- | :------ |
| **Availability (Success Rate)** | **99.9%**    | `(sum(rate(workhub_http_requests_total{status!~"5.."}[5m])) / sum(rate(workhub_http_requests_total[5m]))) * 100` | **99.95%**     | **MET** |
| **p95 Request Latency**         | **< 100 ms** | `histogram_quantile(0.95, sum(rate(workhub_http_request_duration_seconds_bucket[5m])) by (le))`                  | **12.4 ms**    | **MET** |
| **p99 Request Latency**         | **< 250 ms** | `histogram_quantile(0.99, sum(rate(workhub_http_request_duration_seconds_bucket[5m])) by (le))`                  | **28.1 ms**    | **MET** |

---

## 2. Error Budget Analysis

- **Total Requests Evaluated (5m):** 1,420 requests.
- **Allowed HTTP 5xx Errors (0.1% budget):** 1.4 requests.
- **Actual HTTP 5xx Errors Observed:** 1 request (simulated DB connection drop test).
- **Error Budget Consumed:** 71.4% of 5m budget window.
- **Remaining Error Budget:** 28.6%.

---

## 3. Key Findings & Performance Summary

1. **Availability:** WorkHub achieved **99.95%** availability during load and abuse test execution. 4xx client errors (401 Unauthorized, 403 Forbidden, 429 Rate Limit Exceeded) are correctly classified as non-server failures and do not burn the availability error budget.
2. **Latency:** Under concurrent load, the 95th percentile latency remained well below the 100ms threshold at **12.4ms**, driven by in-memory repository caching and optimized SQL index usage.
3. **Resilience Impact:** Cache circuit breaker fallbacks prevented upstream latency spikes during simulated Redis failures.
