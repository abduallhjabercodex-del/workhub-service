# WorkHub One-Page Threat Model (Day 12 Evidence)

This document provides a concise Threat Model analyzing assets, trust boundaries, likely abuse vectors (OWASP Top 10), mitigations, and residual risks in WorkHub.

---

## 🎯 1. Key Assets & Trust Boundaries

### Key Assets
1. **User Credentials & Secrets:** Passwords, bcrypt hashes, JWT Signing Keys (`JWT_SECRET`).
2. **Session Tokens:** Active Access Tokens (JWT) & Opaque Refresh Tokens (SHA-256 Hashes).
3. **Domain Data:** Projects, Tasks, Summaries, Audit Events, and Memberships.

### Trust Boundaries
* **Public Internet ↔ HTTP API Boundary:** Unauthenticated/Untrusted requests crossing into `API.Routes()`.
* **Application Layer ↔ Database Boundary:** Interfacing with PostgreSQL (`Jackc/pgxpool`) and Redis Cache.
* **Service ↔ Worker Boundary:** Event broker publishing background notification jobs.

---

## ⚠️ 2. Threat Analysis & Abuse Vectors (OWASP Top 10)

| Risk Category                                 | Threat / Abuse Vector                                                    | Impact   | Applied Mitigation                                                                                                                                                                                    |
| :-------------------------------------------- | :----------------------------------------------------------------------- | :------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Broken Object Level Auth (BOLA)**           | User A reads/updates tasks belonging to User B's project.                | High     | **Deny-by-Default Policy & RBAC:** `CheckAccess` verifies active membership in `memberships` table for every project resource request. Returns `403 Forbidden`.                                       |
| **Broken Function Level Auth**                | Standard `member` promotes themselves to `owner` or deletes project.     | High     | **Role Hierarchy Checks:** Only `owner` role can grant `admin`/`owner` status or delete project. Verified in `AddProjectMember`.                                                                      |
| **Brute-Force & Denial of Service**           | Attacker floods login or API endpoints with requests/large payloads.     | High     | **Rate Limiting & Payload Limits:** In-memory Token Bucket Rate Limiter (`RateLimit` middleware, 60 req/min) returning `429`, and 1MB payload size limit (`http.MaxBytesReader`) returning `400/413`. |
| **SQL & Command Injection**                   | Malicious SQL inputs passed in query parameters (`q`, `sort`, `id`).     | Critical | **Parameterized SQL Queries:** Exclusive use of `$1, $2` placeholders via `pgx` driver. Zero raw string concatenation for SQL statements.                                                             |
| **Cross-Site Scripting (XSS) & Clickjacking** | Attacker embeds API response in iframe or executes unauthorized scripts. | Medium   | **Security Headers Middleware:** Response headers set `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'self'`.                                      |
| **Server-Side Request Forgery (SSRF)**        | Attacker forces server to make outbound HTTP calls to internal network.  | Medium   | **Input URL Validation:** Server does not accept arbitrary external URLs for fetching resources or webhooks.                                                                                          |

---

## ⚖️ 3. Residual Risks & Next Steps

* **Distributed DoS (DDoS):** While local rate limiting handles single-IP abuse, multi-node deployment should use Cloudflare/AWS WAF or IP-based firewalling at Nginx/Gateway level.
* **Token Storage on Client:** Bearer JWT tokens in browser memory can still be vulnerable if XSS exists in front-end apps. Using `HttpOnly` Secure Cookies is recommended for web frontends.
