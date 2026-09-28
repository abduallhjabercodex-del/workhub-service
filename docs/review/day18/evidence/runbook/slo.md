# WorkHub Service Level Objectives (SLO) & Error Budget Policy

**Document Type:** Production Reliability Specification  
**System:** WorkHub API Platform & Worker Subsystem  
**Measurement Window:** 30-Day Rolling Window  
**Reliability Tier:** Tier-1 Core Business Service  

---

## 1. Service Level Indicators & Objectives (SLI / SLO Definitions)

| Service Level Indicator (SLI)   |          Target (SLO)          | Measurement Window | Underlying Metric / Evaluation Method                                                                                      |
| :------------------------------ | :----------------------------: | :----------------: | :------------------------------------------------------------------------------------------------------------------------- |
| **Availability (Success Rate)** | **$\ge 99.9\%$** (Three Nines) |   30-Day Rolling   | Ratio of non-`5xx` responses: `sum(rate(http_requests_total{status!~"5.."}[30d])) / sum(rate(http_requests_total[30d]))`   |
| **Latency (p95)**               |     **$< 200\text{ ms}$**      |   30-Day Rolling   | 95th percentile request duration: `histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[30d])) by (le))` |
| **Max Tail Latency (p99)**      |     **$< 500\text{ ms}$**      |   30-Day Rolling   | 99th percentile tail latency: `histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[30d])) by (le))`     |
| **Throughput & Capacity**       | **$\ge 5,000\text{ req/sec}$** | Peak Load Windows  | Sustainable traffic handling before activating graceful load shedding                                                      |
| **Background Job Reliability**  |       **$\ge 99.95\%$**        |   30-Day Rolling   | Outbox / worker queue job completion without permanent failure                                                             |

> **Note on Error Exclusions:** Client-side errors (`4xx`, such as invalid inputs, authentication failures, rate limiting `429`, and intentional load shedding `503`) are counted as successful platform enforcement and do not burn the availability error budget.

---

## 2. Error Budget Calculation & Allocations

For a **99.9% Availability SLO**, the allowed error budget is **0.1%** ($1 - 0.999 = 0.001$) of all incoming valid requests over any 30-day evaluation period.

### Quantitative Failure Allowances

* **Monthly Traffic Baseline:** Based on an estimated baseline of **10,000,000 requests/month**:
  $$\text{Allowed Failed Requests} = 10,000,000 \times 0.001 = 10,000\text{ requests}$$
* **Maximum Unscheduled Downtime:**
  $$\text{Allowed Downtime} = 30\text{ days} \times 24\text{ h/day} \times 60\text{ min/day} \times 0.001 = 43.2\text{ minutes/month}$$
  * Per Day: $\approx 1.44\text{ minutes}$
  * Per Week: $\approx 10.08\text{ minutes}$

---

## 3. Multi-Window Multi-Burn-Rate Policy & Incident Actions

To balance fast alerting for catastrophic outages with resilience against transient spikes, WorkHub adopts Google SRE multi-window burn rate thresholds:

|  Burn Rate Multiple   | Budget Consumed | Consumption Window | Severity & Immediate Action                                                                                                       | Notification Channel                  |
| :-------------------: | :-------------: | :----------------: | :-------------------------------------------------------------------------------------------------------------------------------- | :------------------------------------ |
|   **$14.4\times$**    |    **$2\%$**    |       1 Hour       | **Critical (Page On-Call):** Active outage or severe degradation. On-call engineer paged immediately.                             | PagerDuty / Voice Call                |
|    **$6.0\times$**    |    **$5\%$**    |      6 Hours       | **High (Urgent Ticket):** Significant persistent failure rate. Escalated to on-duty team.                                         | Slack `#workhub-alerts` / Jira Urgent |
|    **$1.0\times$**    |   **$10\%$**    |       3 Days       | **Warning (Daily Review):** Steady budget drain. Engineering lead investigates in morning standup.                                | Daily Digest / Slack                  |
| **$100\%$ Exhausted** |   **$100\%$**   |   Current Period   | **Deployment Freeze:** All non-critical feature releases halted. 100% engineering effort redirected to reliability and stability. | Engineering VP Directive              |

---

## 4. PromQL Alerting & Monitoring Rules

```promql
# 1. Immediate Critical Alert: 14.4x burn rate over 1h (2% budget consumed)
(
  sum(rate(http_requests_total{status=~"5.."}[1h])) 
  / 
  sum(rate(http_requests_total[1h]))
) > (14.4 * 0.001)

# 2. Elevated Warning Alert: 6x burn rate over 6h (5% budget consumed)
(
  sum(rate(http_requests_total{status=~"5.."}[6h])) 
  / 
  sum(rate(http_requests_total[6h]))
) > (6.0 * 0.001)

# 3. Latency SLO Alert: p99 exceeds 500ms for > 5 minutes
histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le)) > 0.500
```
