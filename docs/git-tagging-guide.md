# Git Tagging & Release Management Guide — WorkHub Service

This guide provides the complete set of commands and procedures for managing Git release tags, versioning, and binary builds for WorkHub.

---

## 📌 1. Current Release Tags in Repository

Currently, the project contains **2 Release Tags** in chronological sequence:

| Tag Name | Milestone / Description                                                 | Target Commit |
| :------- | :---------------------------------------------------------------------- | :------------ |
| `v0.2.0` | Release v0.2.0 - Day 10 Final Deliverable & Command Handbook            | `2b54848`     |
| `v0.3.0` | Release v0.3.0 - Day 15 Final Deliverable: Security & Resilience Review | `2b54848`     |

---

## 🛠️ 2. Commands Handbook for Managing Tags

### 2.1. Listing & Inspecting Tags

```bash
# List all tags in short format
git tag -l

# List all tags with their release messages
git tag -l -n

# Search for specific tag patterns (e.g. v0.*)
git tag -l "v0.*"

# Inspect detailed metadata of a specific tag (Author, Date, Commit details)
git show v0.3.0
```

---

### 2.2. Creating New Release Tags

```bash
# 1. Create an Annotated Tag for Day 15 / Milestone
git tag -a v0.3.0 -m "Release v0.3.0 - Day 15 Deliverable: Security & Resilience Review (RBAC, Quotas, Circuit Breaker, Observability)"

# 2. Create a Tag on a past specific commit (using Commit Hash)
git tag -a v0.3.1 2b54848 -m "Patch release for commit 2b54848"
```

---

### 2.3. Pushing Tags to Remote Repository (GitHub / GitLab)

```bash
# Push a specific tag to remote
git push origin v0.3.0

# Push ALL local tags to remote at once
git push origin --tags
```

---

### 2.4. Deleting Tags (Local & Remote)

```bash
# Delete a tag from local machine
git tag -d v0.3.0

# Delete a tag from remote repository (GitHub / GitLab)
git push origin --delete v0.3.0
```

---

## 🚀 3. Compiling Go Binary with Embedded Release Version

When building the Go binary for deployment, inject the version tag (`v0.3.0`) and git commit hash directly into `internal/version`:

```bash
# Extract short git commit hash
COMMIT_HASH=$(git rev-parse --short HEAD)

# Compile binary with injected version & commit hash
go build -ldflags "-X workhub-service/internal/version.Version=v0.3.0 -X workhub-service/internal/version.Commit=${COMMIT_HASH}" -o bin/server cmd/server/main.go

# Verify build binary health output
./bin/server
# In another terminal:
curl -s http://localhost:8080/health
# Output: {"status":"ok","version":"v0.3.0","time":"..."}
```

---

## 📋 4. Standard Release Tag Naming Convention

WorkHub adheres to **Semantic Versioning (SemVer)**: `vMAJOR.MINOR.PATCH`

- **`v0.2.0`:** Day 10 Deliverable (Architecture & Operations Baseline).
- **`v0.3.0`:** Day 15 Deliverable (Security, Resilience Review & Audit Package).
- **`v0.4.0`:** Next Milestone Deliverable.
