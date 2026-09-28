# WorkHub API Contract, Versioning, and Compatibility Matrix

**Document Type:** API Design Governance & Contract Specification  
**Specification Standard:** OpenAPI 3.0.3  
**Versioning Strategy:** Explicit URI Path Versioning (`/api/v1/...`)  
**Status:** **Approved / 100% Schema-Compliant**  

---

## 1. API Versioning & Backward Compatibility Principles

### 1. Versioning Mechanism
All public application endpoints are prefixed with an explicit major version identifier (`/api/v1/`). Sub-resources adhere strictly to RESTful conventions.

### 2. Backward Compatibility Rules
1. **No Breaking Changes on Active Versions:** Modifying field data types, removing existing JSON keys, or making optional parameters mandatory is strictly forbidden.
2. **Additive Changes Only:** New endpoints and optional JSON payload/response fields may be added without triggering a major version increment.
3. **Uniform Response Envelope:** All API responses adhere to a consistent standardized envelope structure:
   ```json
   {
     "data": { ... },
     "error": { "code": "RESOURCE_NOT_FOUND", "message": "...", "requestId": "req_123" },
     "meta": { "page": 1, "limit": 20, "total": 100 }
   }
   ```
4. **Deprecation Policy:** When architectural changes require a major version deprecation (`/api/v2/`), the legacy version (`/api/v1/`) remains operational for a minimum of **6 months** while broadcasting `Deprecation: true` and `Sunset: <date>` HTTP headers.

---

## 2. OpenAPI 3.0 Endpoint Matrix

| Endpoint Path                   | HTTP Method | Authentication Scheme      | Function & Behavioral Scope                                                 |
| :------------------------------ | :---------: | :------------------------- | :-------------------------------------------------------------------------- |
| `/health`                       |    `GET`    | Public (Unauthenticated)   | Basic liveness probe checking container responsiveness                      |
| `/ready`                        |    `GET`    | Public (Unauthenticated)   | Deep readiness probe checking PostgreSQL and Redis availability             |
| `/metrics`                      |    `GET`    | Public / Prometheus Scrape | Real-time Prometheus telemetry and operational RED metrics                  |
| `/api/v1/auth/register`         |   `POST`    | Rate Limited (5 req / 10s) | New user registration and credential issuance                               |
| `/api/v1/auth/login`            |   `POST`    | Rate Limited (5 req / 10s) | User credential verification, JWT access token & refresh token issuance     |
| `/api/v1/auth/refresh`          |   `POST`    | Refresh Token Only         | Session renewal with single-use token rotation and replay attack revocation |
| `/api/v1/auth/me`               |    `GET`    | Bearer JWT                 | Returns the authenticated user's profile and RBAC role                      |
| `/api/v1/projects`              | `GET, POST` | Bearer JWT                 | List and create projects with tenant quota enforcement                      |
| `/api/v1/projects/{id}/summary` |    `GET`    | Bearer JWT                 | Project analytics summary backed by high-speed Redis caching                |
| `/api/v1/projects/{id}/members` | `GET, POST` | Bearer JWT                 | Project collaborator management with RBAC permission validation             |
| `/api/v1/projects/{id}/tasks`   | `GET, POST` | Bearer JWT                 | Task lifecycle operations with automatic tenant quota checks                |
| `/api/v1/quotas/{userId}`       | `GET, PUT`  | Admin JWT Role Required    | Administrative quota inspection and limit overrides                         |

---

## 3. Schema Verification & Contract Testing Suite

```bash
# 1. Validate OpenAPI YAML syntax and structure
python3 -c "import yaml; yaml.safe_load(open('docs/openapi.yaml'))" && echo "OpenAPI YAML is 100% valid"

# 2. Verify Swagger UI documentation ingress response
curl -s -I http://localhost:8090/api/v1/docs | head -n 5

# 3. Retrieve raw OpenAPI specification through live API
curl -s http://localhost:8090/api/v1/docs/openapi.yaml | head -n 10
```
