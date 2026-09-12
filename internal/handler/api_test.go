package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

func testAPI() http.Handler {
	repo := repository.NewMemoryRepository()
	return NewAPI(service.NewWorkHub(repo, repo)).Routes()
}

func TestProjectAndTaskFlow(t *testing.T) {
	api := testAPI()
	projectReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"Demo"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectRes := httptest.NewRecorder()
	api.ServeHTTP(projectRes, projectReq)
	if projectRes.Code != http.StatusCreated {
		t.Fatalf("project status = %d", projectRes.Code)
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
	patchRes := httptest.NewRecorder()
	api.ServeHTTP(patchRes, patchReq)
	if patchRes.Code != http.StatusOK {
		t.Fatalf("patch status = %d", patchRes.Code)
	}
}

func TestValidationErrorEnvelope(t *testing.T) {
	api := testAPI()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", res.Code)
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"invalid_input"`)) {
		t.Fatalf("unexpected body: %s", res.Body.String())
	}
}
