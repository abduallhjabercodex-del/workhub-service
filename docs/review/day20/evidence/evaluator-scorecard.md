# Official Evaluator Scorecard & Qualification Decision Record

**Project:** WorkHub Enterprise Modular Monolith  
**Evaluation Scope:** Day 20 Final Qualification & Weekly Task 4  
**Candidate:** Engineering Candidate  
**Evaluator:** _______________________________________  
**Evaluation Date:** _________________________________  

---

## 1. Overall Scoring Table

*Passing Threshold:* Overall score $\ge 70 / 100$ **AND** All 6 Mandatory Gates marked as **PASS**.

| Task Milestone | Evaluated Focus | Weight | Maximum Score | Evaluator Score |
|---|---|:---:|:---:|:---:|
| **Weekly Task 1** | Go Architecture, REST APIs, Domain Modeling, Basic Unit Testing | 15% | 15 | `[      ] / 15` |
| **Weekly Task 2** | Relational SQL, ACID Transactions, Outbox, Redis Cache, Workers | 20% | 20 | `[      ] / 20` |
| **Weekly Task 3** | Dual-Token Auth, RBAC Authorization, Rate Limiting, Observability | 25% | 25 | `[      ] / 25` |
| **Weekly Task 4** | Containers, CI/CD, Profiling, Disaster Recovery, PRR, System Defense | 40% | 40 | `[      ] / 40` |
| **TOTAL** | **Comprehensive Engineering & Operational Score** | **100%** | **100** | **`[      ] / 100`** |

---

## 2. Mandatory Gates Verification

> [!CAUTION]
> **Strict Prerequisite:** A failure in **any single mandatory gate** constitutes an immediate, unconditional **FAIL** for the entire qualification review, regardless of the numerical score above.

| Gate # | Safety & Architecture Threshold | Verified Requirement | Gate Decision |
|:---:|---|---|:---:|
| **Gate 1** | **Secrets & Credential Hygiene** | Zero committed credentials/keys; bcrypt cost $\ge 12$; structured log token redaction. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |
| **Gate 2** | **Authorization & Zero IDOR** | 4-tier RBAC (`owner > admin > member > viewer`); zero cross-tenant data leakage. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |
| **Gate 3** | **Clean Migrations & DR Drill** | Deterministic forward migrations; proven backup/restore with checksum validation. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |
| **Gate 4** | **Concurrency & Code Quality** | Clean formatting (`gofmt`); zero vet issues; zero data races under `go test -race`. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |
| **Gate 5** | **Operational Lifecycle** | Accurate `/health` and `/ready` probes; zero-drop graceful shutdown under SIGTERM. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |
| **Gate 6** | **Zero PRR Critical Defects** | Zero crash loops; zero silent data corruption; 0 open critical PRR defects. | `[ ] PASS` &nbsp;&nbsp; `[ ] FAIL` |

---

## 3. Evaluation Criteria Rubric

To maintain objective evaluation standards, evaluators should adhere to the following scoring guidelines:

### Weekly Task 1 (Max 15 pts):
* **13–15 pts:** Clean modular architecture (`handler → service → repository`); clear domain models; structured error responses; consistent filtering & pagination.
* **10–12 pts:** Working endpoints with minor coupling between HTTP handlers and database logic.
* **< 10 pts:** God objects, global variables, missing domain boundaries.

### Weekly Task 2 (Max 20 pts):
* **18–20 pts:** Parameterized queries preventing SQL injection; explicit PostgreSQL transactions with rollback tests; idempotent worker processing; cache-aside with TTL.
* **14–17 pts:** Basic CRUD working in Postgres, but lacking comprehensive transaction rollback proofs or worker deduplication.
* **< 14 pts:** SQL injection vulnerabilities, non-transactional compound writes, or unhandled broker disconnections.

### Weekly Task 3 (Max 25 pts):
* **22–25 pts:** Dual-token JWT with rotating refresh tokens; replay attack family revocation; strict RBAC denial-by-default; token-bucket rate limiting; Prometheus RED/USE metrics and structured JSON logs.
* **18–21 pts:** Basic JWT auth and roles, but refresh token reuse is not prevented or rate limiting lacks standard HTTP headers.
* **< 18 pts:** Insecure passwords, IDOR leaks, or missing audit logs.

### Weekly Task 4 (Max 40 pts):
* **36–40 pts:** Production-ready multi-stage Dockerfiles with unprivileged users; automated 7-stage CI pipeline; baseline load tests with pprof profiling; proven backup/restore drill with verified RTO/RPO; comprehensive ADRs; live operational chaos demonstration; deep RCA of injected defects.
* **28–35 pts:** Working containers and tests, but profiling analysis is superficial or disaster recovery drill lacks checksum validation.
* **< 28 pts:** Containers run as root, unhandled CI failures, or missing operational runbooks.

---

## 4. Evaluator Final Decision & Sign-Off

### Decision Checkboxes:
- [ ] **PASS:** The candidate has demonstrated comprehensive mastery of production backend engineering, achieved a score $\ge 70\%$, and satisfied all 6 mandatory gates without reservation.
- [ ] **CONDITIONAL PASS:** The candidate scored $\ge 70\%$ and passed all mandatory gates, but must remediate minor non-critical documentation or non-blocking items within 48 hours.
- [ ] **FAIL:** The candidate failed to meet the passing threshold ($< 70\%$) OR failed one or more mandatory gates.

### Evaluator Comments & Technical Feedback:
```text
Strengths:
____________________________________________________________________________________
____________________________________________________________________________________
____________________________________________________________________________________

Areas for Continued Growth:
____________________________________________________________________________________
____________________________________________________________________________________
____________________________________________________________________________________
```

### Signatures:

**Lead Evaluator Signature:** _______________________________________ &nbsp;&nbsp;&nbsp;&nbsp; **Date:** _______________  
**Engineering Director Signature:** ________________________________ &nbsp;&nbsp;&nbsp;&nbsp; **Date:** _______________  
