# WorkHub Authorization Matrix (Day 12 Evidence)

This document defines the Role-Based Access Control (RBAC) policy, resource ownership boundaries, and deny-by-default rules enforced across the WorkHub API.

---

## 👥 Roles & Permissions Hierarchy

* **Owner (`owner`):** Full administrative control over the project. Created automatically when a user creates a project.
* **Admin (`admin`):** Project management role. Can add/remove regular members, manage tasks, and view summaries.
* **Member (`member`):** Standard project contributor. Can view the project, create/update/delete tasks, and view summaries.
* **Viewer (`viewer`):** Read-only participant. Can view projects, tasks, and summaries, but cannot modify any resources.
* **Non-Member:** Any authenticated user who has NOT joined or been added to the target project.

---

## 📊 Permission Matrix Table

| Operation / Resource     | Endpoint                                     | Owner |      Admin       | Member | Viewer |  Non-Member   | Unauthenticated |
| :----------------------- | :------------------------------------------- | :---: | :--------------: | :----: | :----: | :-----------: | :-------------: |
| **Create Project**       | `POST /api/v1/projects`                      |  ✅   |        ✅        |   ✅   |   ✅   |      ✅       |     ❌ 401      |
| **List User Projects**   | `GET /api/v1/projects`                       |  ✅   |        ✅        |   ✅   |   ✅   | ❌ (Filtered) |     ❌ 401      |
| **Get Project Summary**  | `GET /api/v1/projects/{id}/summary`          |  ✅   |        ✅        |   ✅   |   ✅   |    ❌ 403     |     ❌ 401      |
| **List Project Members** | `GET /api/v1/projects/{id}/members`          |  ✅   |        ✅        |   ✅   |   ✅   |    ❌ 403     |     ❌ 401      |
| **Add Project Member**   | `POST /api/v1/projects/{id}/members`         |  ✅   | ✅ (Member only) | ❌ 403 | ❌ 403 |    ❌ 403     |     ❌ 401      |
| **Promote/Assign Admin** | `POST /api/v1/projects/{id}/members`         |  ✅   |      ❌ 403      | ❌ 403 | ❌ 403 |    ❌ 403     |     ❌ 401      |
| **Remove Member**        | `DELETE /api/v1/projects/{id}/members/{uId}` |  ✅   |        ✅        | ❌ 403 | ❌ 403 |    ❌ 403     |     ❌ 401      |
| **List Project Tasks**   | `GET /api/v1/projects/{id}/tasks`            |  ✅   |        ✅        |   ✅   |   ✅   |    ❌ 403     |     ❌ 401      |
| **Create Task**          | `POST /api/v1/projects/{id}/tasks`           |  ✅   |        ✅        |   ✅   | ❌ 403 |    ❌ 403     |     ❌ 401      |
| **Update Task Status**   | `PATCH /api/v1/tasks/{id}`                   |  ✅   |        ✅        |   ✅   | ❌ 403 |    ❌ 403     |     ❌ 401      |
| **Delete Task**          | `DELETE /api/v1/tasks/{id}`                  |  ✅   |        ✅        |   ✅   | ❌ 403 |    ❌ 403     |     ❌ 401      |

---

## 🛡️ Anti-Privilege Escalation Controls

### 1. Horizontal Privilege Escalation Protection (BOLA/BOC)
* **Threat:** User A (Attacker) accesses User B's project or tasks by guessing/passing Project ID in request.
* **Mitigation:** Enforced in `CheckAccess` service layer and repository filtering. Requests for project resources check if `actorUserID` exists in `memberships` table for that `project_id`. If absent, returned response is `403 Forbidden` (Deny-by-Default).

### 2. Vertical Privilege Escalation Protection
* **Threat:** A standard `member` attempts to add themselves or others as `admin`/`owner`, or call administrative APIs.
* **Mitigation:** Only `owner` can assign `admin` or `owner` roles. Attempts by regular `member` or `admin` to promote to `owner` result in `403 Forbidden`.
