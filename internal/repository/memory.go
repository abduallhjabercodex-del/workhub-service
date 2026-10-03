package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"workhub-service/internal/domain"
)

type MemoryRepository struct {
	mu            sync.RWMutex
	projects      map[string]domain.Project
	tasks         map[string]domain.Task
	users         map[string]domain.User
	usersByEmail  map[string]domain.User
	tokens        map[string]domain.RefreshTokenSession
	members       map[string]map[string]domain.Member // projectID -> userID -> Member
	userQuotas    map[string]domain.UserQuota
	auditEvents   []domain.AuditEvent
	outboxEvents  []domain.OutboxRecord
	nextUserID    int64
	nextTokenID   int64
	nextProjectID int64
	nextTaskID    int64
	nextAuditID   int64
	nextOutboxID  int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		projects:      map[string]domain.Project{},
		tasks:         map[string]domain.Task{},
		users:         map[string]domain.User{},
		usersByEmail:  map[string]domain.User{},
		tokens:        map[string]domain.RefreshTokenSession{},
		members:       map[string]map[string]domain.Member{},
		userQuotas:    map[string]domain.UserQuota{},
		auditEvents:   []domain.AuditEvent{},
		outboxEvents:  []domain.OutboxRecord{},
		nextUserID:    1,
		nextTokenID:   1,
		nextProjectID: 1,
		nextTaskID:    1,
		nextAuditID:   1,
		nextOutboxID:  1,
	}
}

func (r *MemoryRepository) Ping(_ context.Context) error {
	return nil
}

func (r *MemoryRepository) CreateProject(_ context.Context, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = strconv.FormatInt(r.nextProjectID, 10)
		r.nextProjectID++
	}
	r.projects[p.ID] = p
	if p.OwnerID != "" {
		if _, ok := r.members[p.ID]; !ok {
			r.members[p.ID] = map[string]domain.Member{}
		}
		u, _ := r.users[p.OwnerID]
		r.members[p.ID][p.OwnerID] = domain.Member{
			ProjectID:   p.ID,
			UserID:      p.OwnerID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Role:        domain.RoleOwner,
			JoinedAt:    time.Now().UTC(),
		}
	}
	return p, nil
}

func (r *MemoryRepository) ListProjects(_ context.Context, userID, q, sortBy string, limit, offset int) ([]domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Project, 0, len(r.projects))
	for _, p := range r.projects {
		if userID != "" {
			// Check if user is owner or member
			isMember := p.OwnerID == userID
			if projMembers, ok := r.members[p.ID]; ok {
				if _, found := projMembers[userID]; found {
					isMember = true
				}
			}
			if !isMember {
				continue
			}
		}
		if q == "" || containsFold(p.Name, q) {
			items = append(items, p)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if sortBy == "name" {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	total := len(items)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []domain.Project{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end], nil
}

func (r *MemoryRepository) GetProject(_ context.Context, id string) (domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projects[id]
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) DeleteProject(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.projects[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.projects, id)
	delete(r.members, id)
	return nil
}

func (r *MemoryRepository) CreateTask(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.projects[t.ProjectID]; !ok {
		return domain.Task{}, ErrNotFound
	}
	if t.ID == "" {
		t.ID = strconv.FormatInt(r.nextTaskID, 10)
		r.nextTaskID++
	}
	r.tasks[t.ID] = t
	return t, nil
}

func (r *MemoryRepository) ListTasks(_ context.Context, projectID, status, q, sortBy string, limit, offset int) ([]domain.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var filtered []domain.Task
	for _, t := range r.tasks {
		if projectID != "" && t.ProjectID != projectID {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		if q != "" && !containsFold(t.Title, q) {
			continue
		}
		filtered = append(filtered, t)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if sortBy == "title" {
			return strings.ToLower(filtered[i].Title) < strings.ToLower(filtered[j].Title)
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	if offset >= total {
		return []domain.Task{}, total, nil
	}
	end := offset + limit
	if end > total || limit <= 0 {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (r *MemoryRepository) GetProjectSummary(_ context.Context, projectID string) (domain.ProjectSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.projects[projectID]; !ok {
		return domain.ProjectSummary{}, ErrNotFound
	}
	var summary domain.ProjectSummary
	summary.ProjectID = projectID
	for _, task := range r.tasks {
		if task.ProjectID != projectID {
			continue
		}
		summary.TaskCount++
		if task.Status == "done" {
			summary.CompletedCount++
		} else {
			summary.OpenCount++
		}
	}
	return summary, nil
}

func (r *MemoryRepository) GetTask(_ context.Context, id string) (domain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return domain.Task{}, ErrNotFound
	}
	return t, nil
}

func (r *MemoryRepository) UpdateTask(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[t.ID]; !ok {
		return domain.Task{}, ErrNotFound
	}
	r.tasks[t.ID] = t
	return t, nil
}

func (r *MemoryRepository) DeleteTask(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(r.tasks, id)
	return nil
}

func (r *MemoryRepository) CreateUser(_ context.Context, email, displayName, passwordHash string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	normEmail := strings.ToLower(strings.TrimSpace(email))
	if _, exists := r.usersByEmail[normEmail]; exists {
		return domain.User{}, ErrAlreadyExists
	}

	id := strconv.FormatInt(r.nextUserID, 10)
	r.nextUserID++
	now := time.Now().UTC()

	user := domain.User{
		ID:           id,
		Email:        normEmail,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    now,
	}

	r.users[id] = user
	r.usersByEmail[normEmail] = user
	return user, nil
}

func (r *MemoryRepository) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normEmail := strings.ToLower(strings.TrimSpace(email))
	user, exists := r.usersByEmail[normEmail]
	if !exists {
		return domain.User{}, ErrNotFound
	}
	return user, nil
}

func (r *MemoryRepository) GetUserByID(_ context.Context, id string) (domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return domain.User{}, ErrNotFound
	}
	return user, nil
}

func (r *MemoryRepository) SaveRefreshToken(_ context.Context, session domain.RefreshTokenSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tokens[session.TokenHash]; exists {
		return ErrAlreadyExists
	}
	if session.ID == "" {
		session.ID = strconv.FormatInt(r.nextTokenID, 10)
		r.nextTokenID++
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	r.tokens[session.TokenHash] = session
	return nil
}

func (r *MemoryRepository) GetRefreshToken(_ context.Context, tokenHash string) (domain.RefreshTokenSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	token, exists := r.tokens[tokenHash]
	if !exists {
		return domain.RefreshTokenSession{}, ErrNotFound
	}
	return token, nil
}

func (r *MemoryRepository) RevokeRefreshToken(_ context.Context, tokenHash string, replacedByHash *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	token, exists := r.tokens[tokenHash]
	if !exists || token.RevokedAt != nil {
		return ErrNotFound
	}

	now := time.Now().UTC()
	token.RevokedAt = &now
	token.ReplacedByTokenHash = replacedByHash
	r.tokens[tokenHash] = token
	return nil
}

func (r *MemoryRepository) RevokeAllUserRefreshTokens(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for hash, token := range r.tokens {
		if token.UserID == userID && token.RevokedAt == nil {
			token.RevokedAt = &now
			r.tokens[hash] = token
		}
	}
	return nil
}

func (r *MemoryRepository) RotateRefreshToken(_ context.Context, oldHash string, newSession domain.RefreshTokenSession) (domain.RefreshTokenSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	oldToken, exists := r.tokens[oldHash]
	if !exists {
		return domain.RefreshTokenSession{}, ErrNotFound
	}
	if oldToken.RevokedAt != nil {
		// Token Replay Detected! Revoke all tokens for this user
		now := time.Now().UTC()
		for hash, token := range r.tokens {
			if token.UserID == oldToken.UserID && token.RevokedAt == nil {
				token.RevokedAt = &now
				r.tokens[hash] = token
			}
		}
		return domain.RefreshTokenSession{}, domain.ErrTokenReplay
	}
	if time.Now().UTC().After(oldToken.ExpiresAt) {
		return domain.RefreshTokenSession{}, ErrNotFound
	}

	// Revoke old token
	now := time.Now().UTC()
	oldToken.RevokedAt = &now
	oldToken.ReplacedByTokenHash = &newSession.TokenHash
	r.tokens[oldHash] = oldToken

	// Save new token
	newSession.ID = strconv.FormatInt(r.nextTokenID, 10)
	r.nextTokenID++
	if newSession.CreatedAt.IsZero() {
		newSession.CreatedAt = now
	}
	r.tokens[newSession.TokenHash] = newSession

	return newSession, nil
}

func (r *MemoryRepository) AddProjectMember(_ context.Context, projectID, userID string, role domain.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.projects[projectID]; !ok {
		return ErrNotFound
	}
	u, ok := r.users[userID]
	if !ok {
		return ErrNotFound
	}
	if role == "" {
		role = domain.RoleMember
	}
	if _, ok := r.members[projectID]; !ok {
		r.members[projectID] = map[string]domain.Member{}
	}
	r.members[projectID][userID] = domain.Member{
		ProjectID:   projectID,
		UserID:      userID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        role,
		JoinedAt:    time.Now().UTC(),
	}
	return nil
}

func (r *MemoryRepository) GetProjectMember(_ context.Context, projectID, userID string) (domain.Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pMembers, ok := r.members[projectID]
	if !ok {
		return domain.Member{}, ErrNotFound
	}
	m, ok := pMembers[userID]
	if !ok {
		return domain.Member{}, ErrNotFound
	}
	return m, nil
}

func (r *MemoryRepository) ListProjectMembers(_ context.Context, projectID string) ([]domain.Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.projects[projectID]; !ok {
		return nil, ErrNotFound
	}
	pMembers, ok := r.members[projectID]
	if !ok {
		return []domain.Member{}, nil
	}
	list := make([]domain.Member, 0, len(pMembers))
	for _, m := range pMembers {
		list = append(list, m)
	}
	return list, nil
}

func (r *MemoryRepository) RemoveProjectMember(_ context.Context, projectID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pMembers, ok := r.members[projectID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := pMembers[userID]; !ok {
		return ErrNotFound
	}
	delete(pMembers, userID)
	return nil
}

func (r *MemoryRepository) RemoveProjectMemberSafely(ctx context.Context, projectID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pMembers, ok := r.members[projectID]
	if !ok {
		return ErrNotFound
	}
	m, ok := pMembers[userID]
	if !ok {
		return ErrNotFound
	}

	if m.Role == domain.RoleOwner {
		ownerCount := 0
		for _, member := range pMembers {
			if member.Role == domain.RoleOwner {
				ownerCount++
			}
		}
		if ownerCount <= 1 {
			return domain.ErrLastOwner
		}
	}

	delete(pMembers, userID)
	return nil
}

func (r *MemoryRepository) CreateProjectWithQuota(_ context.Context, p domain.Project, maxProjects int) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, proj := range r.projects {
		if proj.OwnerID == p.OwnerID {
			count++
		}
	}
	if maxProjects >= 0 && count >= maxProjects {
		return domain.Project{}, &domain.QuotaError{Resource: "projects", Current: count, Max: maxProjects}
	}

	if p.ID == "" {
		p.ID = strconv.FormatInt(r.nextProjectID, 10)
		r.nextProjectID++
	}
	r.projects[p.ID] = p
	if p.OwnerID != "" {
		if _, ok := r.members[p.ID]; !ok {
			r.members[p.ID] = map[string]domain.Member{}
		}
		u, _ := r.users[p.OwnerID]
		r.members[p.ID][p.OwnerID] = domain.Member{
			ProjectID:   p.ID,
			UserID:      p.OwnerID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Role:        domain.RoleOwner,
			JoinedAt:    time.Now().UTC(),
		}
	}
	return p, nil
}

func (r *MemoryRepository) CountUserProjects(_ context.Context, ownerID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, proj := range r.projects {
		if proj.OwnerID == ownerID {
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) CreateTaskWithQuota(_ context.Context, t domain.Task, audit domain.AuditEvent, maxTasks int) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.projects[t.ProjectID]; !ok {
		return domain.Task{}, ErrNotFound
	}
	count := 0
	for _, task := range r.tasks {
		if task.ProjectID == t.ProjectID {
			count++
		}
	}
	if maxTasks >= 0 && count >= maxTasks {
		return domain.Task{}, &domain.QuotaError{Resource: "tasks", Current: count, Max: maxTasks}
	}

	if t.ID == "" {
		t.ID = strconv.FormatInt(r.nextTaskID, 10)
		r.nextTaskID++
	}
	r.tasks[t.ID] = t

	payload, _ := json.Marshal(map[string]any{
		"taskId":    t.ID,
		"projectId": t.ProjectID,
		"title":     t.Title,
		"status":    t.Status,
	})
	r.outboxEvents = append(r.outboxEvents, domain.OutboxRecord{
		ID:            r.nextOutboxID,
		EventID:       fmt.Sprintf("task-created-%s-%d", t.ID, time.Now().UnixNano()),
		EventType:     "task.created",
		AggregateType: "task",
		AggregateID:   t.ID,
		Payload:       payload,
		Attempts:      0,
		AvailableAt:   time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	})
	r.nextOutboxID++

	return t, nil
}

func (r *MemoryRepository) CountProjectTasks(_ context.Context, projectID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, task := range r.tasks {
		if task.ProjectID == projectID {
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) AddProjectMemberWithQuota(_ context.Context, projectID, userID string, role domain.Role, maxMembers int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.projects[projectID]; !ok {
		return ErrNotFound
	}
	u, ok := r.users[userID]
	if !ok {
		return ErrNotFound
	}
	if role == "" {
		role = domain.RoleMember
	}
	pMembers, ok := r.members[projectID]
	if !ok {
		pMembers = map[string]domain.Member{}
		r.members[projectID] = pMembers
	}

	_, exists := pMembers[userID]
	if !exists && maxMembers >= 0 && len(pMembers) >= maxMembers {
		return &domain.QuotaError{Resource: "members", Current: len(pMembers), Max: maxMembers}
	}

	pMembers[userID] = domain.Member{
		ProjectID:   projectID,
		UserID:      userID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        role,
		JoinedAt:    time.Now().UTC(),
	}
	return nil
}

func (r *MemoryRepository) CountProjectMembers(_ context.Context, projectID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pMembers, ok := r.members[projectID]
	if !ok {
		return 0, nil
	}
	return len(pMembers), nil
}

func (r *MemoryRepository) GetUserQuota(_ context.Context, userID string) (domain.UserQuota, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	quota, ok := r.userQuotas[userID]
	if !ok {
		return domain.UserQuota{UserID: userID}, false, nil
	}
	return quota, true, nil
}

func (r *MemoryRepository) SetUserQuota(_ context.Context, quota domain.UserQuota) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userQuotas[quota.UserID] = quota
	return nil
}

func (r *MemoryRepository) ListProjectAuditEvents(_ context.Context, projectID string, limit, offset int) ([]domain.AuditEvent, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var filtered []domain.AuditEvent
	for _, a := range r.auditEvents {
		if a.ProjectID == projectID {
			filtered = append(filtered, a)
		}
	}
	total := len(filtered)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []domain.AuditEvent{}, total, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (r *MemoryRepository) RecordAuditEvent(_ context.Context, event domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if event.ID == "" {
		event.ID = strconv.FormatInt(r.nextAuditID, 10)
		r.nextAuditID++
	}
	if user, ok := r.users[event.ActorUserID]; ok {
		event.ActorDisplayName = user.DisplayName
		event.ActorEmail = user.Email
	}
	r.auditEvents = append([]domain.AuditEvent{event}, r.auditEvents...)
	return nil
}

func containsFold(value, query string) bool {
	return len(query) == 0 || len(value) >= len(query) && indexFold(value, query)
}

func indexFold(value, query string) bool {
	for i := 0; i+len(query) <= len(value); i++ {
		match := true
		for j := range query {
			if lower(value[i+j]) != lower(query[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func (r *MemoryRepository) FetchUnpublishedOutboxEvents(_ context.Context, batchSize int) ([]domain.OutboxRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	var result []domain.OutboxRecord
	for _, rec := range r.outboxEvents {
		if rec.PublishedAt == nil && !rec.AvailableAt.After(now) {
			result = append(result, rec)
			if batchSize > 0 && len(result) >= batchSize {
				break
			}
		}
	}
	return result, nil
}

func (r *MemoryRepository) MarkOutboxEventPublished(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for i := range r.outboxEvents {
		if r.outboxEvents[i].ID == id {
			r.outboxEvents[i].PublishedAt = &now
			r.outboxEvents[i].LastError = nil
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *MemoryRepository) MarkOutboxEventFailed(_ context.Context, id int64, lastErr string, nextRetry time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	next := time.Now().UTC().Add(nextRetry)
	for i := range r.outboxEvents {
		if r.outboxEvents[i].ID == id {
			r.outboxEvents[i].Attempts++
			r.outboxEvents[i].LastError = &lastErr
			r.outboxEvents[i].AvailableAt = next
			return nil
		}
	}
	return domain.ErrNotFound
}
