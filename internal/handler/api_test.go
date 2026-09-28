package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

const testSecret = "test-jwt-secret-key-at-least-32-chars-long!"

func testAPI() (*API, http.Handler, string) {
	repo := repository.NewMemoryRepository()
	deps := service.Deps{
		Projects:    repo,
		Tasks:       repo,
		Users:       repo,
		Auth:        repo,
		Memberships: repo,
		Config: service.AuthConfig{
			JWTSecret:       testSecret,
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	s, err := service.NewWorkHub(deps)
	if err != nil {
		panic(err)
	}

	user, _ := s.Register(nil, domain.RegisterInput{
		Email:       "test@example.com",
		DisplayName: "Test User",
		Password:    "SecurePassword123",
	})

	api := NewAPI(s, nil, nil, slog.Default())
	handler := middleware.Authenticate(testSecret)(api.Routes())
	return api, handler, user.AccessToken
}

// TestProjectAndTaskFlow يختبر سير العمل التكاملي لإنشاء مشروع جديد، ثم إضافة مهمة فرعية وتحديث حالتها إلى (done) بنجاح عبر الواجهة البرمجية HTTP.
func TestProjectAndTaskFlow(t *testing.T) {
	_, api, token := testAPI()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"Demo"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.Header.Set("Authorization", "Bearer "+token)
	projectRes := httptest.NewRecorder()
	api.ServeHTTP(projectRes, projectReq)
	if projectRes.Code != http.StatusCreated {
		t.Fatalf("project status = %d, body = %s", projectRes.Code, projectRes.Body.String())
	}

	var project struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(projectRes.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.Data.ID == "" {
		t.Fatal("project id is empty")
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.Data.ID+"/tasks", bytes.NewBufferString(`{"title":"First task"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.Header.Set("Authorization", "Bearer "+token)
	taskRes := httptest.NewRecorder()
	api.ServeHTTP(taskRes, taskReq)
	if taskRes.Code != http.StatusCreated {
		t.Fatalf("task status = %d, body = %s", taskRes.Code, taskRes.Body.String())
	}

	var task struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(taskRes.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+task.Data.ID, bytes.NewBufferString(`{"status":"done"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchRes := httptest.NewRecorder()
	api.ServeHTTP(patchRes, patchReq)
	if patchRes.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchRes.Code, patchRes.Body.String())
	}
}

// TestValidationErrorEnvelope يختبر إرجاع غلاف الأخطاء الموحد (Error Envelope) برموز الاستجابة المناسبة (400 Bad Request) عند تقديم بيانات غير صالحة.
func TestValidationErrorEnvelope(t *testing.T) {
	_, api, token := testAPI()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	api.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"invalid_input"`)) {
		t.Fatalf("unexpected body: %s", res.Body.String())
	}
}

// TestHealthAndReadinessEndpoints يختبر نقاط النهاية الخاصة بفحص السلامة وحالة جاهزية النظام (/health و /ready) للتأكد من إرجاع حالة 200 OK.
func TestHealthAndReadinessEndpoints(t *testing.T) {
	rawAPI, api, _ := testAPI()

	// Need to manually set these to true since main() usually does this
	rawAPI.AppHealth().MarkReady(true)
	rawAPI.AppHealth().MarkStartupComplete(true)

	// Test /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	resHealth := httptest.NewRecorder()
	api.ServeHTTP(resHealth, reqHealth)
	if resHealth.Code != http.StatusOK {
		t.Fatalf("health status = %d, expected %d", resHealth.Code, http.StatusOK)
	}

	// Test /ready
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	resReady := httptest.NewRecorder()
	api.ServeHTTP(resReady, reqReady)
	if resReady.Code != http.StatusOK {
		t.Fatalf("ready status = %d, expected %d", resReady.Code, http.StatusOK)
	}

	// Test /startup
	reqStartup := httptest.NewRequest(http.MethodGet, "/startup", nil)
	resStartup := httptest.NewRecorder()
	api.ServeHTTP(resStartup, reqStartup)
	if resStartup.Code != http.StatusOK {
		t.Fatalf("startup status = %d, expected %d", resStartup.Code, http.StatusOK)
	}
}
