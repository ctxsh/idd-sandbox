package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unionai/idd-sandbox/internal/app"
	"github.com/unionai/idd-sandbox/pkg/deployments"
)

func TestHandlerPostDeploymentsSuccess(t *testing.T) {
	useCase := &fakeDeploymentsUseCase{
		deployment: validDeployment(),
	}
	handler := NewHandler(useCase)

	resp, body := httpRequest(t, handler, http.MethodPost, "/deployments", validCreateJSON())

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusCreated)
	}
	var deployment deploymentResponse
	decodeJSON(t, body, &deployment)
	if deployment.ID != "dep_123" {
		t.Fatalf("ID = %q, want dep_123", deployment.ID)
	}
	if deployment.Status != deployments.StatusPendingApproval {
		t.Fatalf("Status = %q, want %q", deployment.Status, deployments.StatusPendingApproval)
	}
	if useCase.create.Service != "payments" {
		t.Fatalf("created service = %q, want payments", useCase.create.Service)
	}
}

func TestHandlerPostDeploymentsInvalidCreate(t *testing.T) {
	useCase := &fakeDeploymentsUseCase{
		createErr: &deployments.ValidationError{Fields: map[string]string{"service": "is required"}},
	}
	handler := NewHandler(useCase)

	resp, body := httpRequest(t, handler, http.MethodPost, "/deployments", `{}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusBadRequest)
	}
	var parsed struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeJSON(t, body, &parsed)
	if parsed.Error != "validation_error" {
		t.Fatalf("error = %q, want validation_error", parsed.Error)
	}
	if parsed.Fields["service"] == "" {
		t.Fatalf("service validation missing in %#v", parsed.Fields)
	}
}

func TestHandlerPostDeploymentsRejectsGeneratedFields(t *testing.T) {
	handler := NewHandler(&fakeDeploymentsUseCase{})

	body := strings.TrimSuffix(validCreateJSON(), "}") + `,"id":"client-owned"}`
	resp, responseBody := httpRequest(t, handler, http.MethodPost, "/deployments", body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, responseBody, http.StatusBadRequest)
	}
	var parsed struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeJSON(t, responseBody, &parsed)
	if parsed.Fields["body"] != `unknown field "id"` {
		t.Fatalf("body validation = %q, want unknown id field", parsed.Fields["body"])
	}
}

func TestHandlerGetDeploymentSuccessAndNotFound(t *testing.T) {
	handler := NewHandler(&fakeDeploymentsUseCase{
		deployment: validDeployment(),
	})

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments/dep_123", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var found deploymentResponse
	decodeJSON(t, body, &found)
	if found.ID != "dep_123" {
		t.Fatalf("found ID = %q, want dep_123", found.ID)
	}

	notFoundHandler := NewHandler(&fakeDeploymentsUseCase{findErr: app.ErrNotFound})
	notFoundResp, notFoundBody := httpRequest(t, notFoundHandler, http.MethodGet, "/deployments/missing", "")
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want %d", notFoundResp.StatusCode, notFoundBody, http.StatusNotFound)
	}
	assertNotFoundBody(t, notFoundBody)
}

func TestHandlerGetDeploymentEventsSuccessAndNotFound(t *testing.T) {
	handler := NewHandler(&fakeDeploymentsUseCase{
		events: []deployments.DeploymentEvent{validEvent()},
	})

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments/dep_123/events", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var events []eventResponse
	decodeJSON(t, body, &events)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Type != deployments.EventDeploymentCreated {
		t.Fatalf("event Type = %q, want %q", events[0].Type, deployments.EventDeploymentCreated)
	}

	notFoundHandler := NewHandler(&fakeDeploymentsUseCase{eventsErr: app.ErrNotFound})
	notFoundResp, notFoundBody := httpRequest(t, notFoundHandler, http.MethodGet, "/deployments/missing/events", "")
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want %d", notFoundResp.StatusCode, notFoundBody, http.StatusNotFound)
	}
	assertNotFoundBody(t, notFoundBody)
}

type fakeDeploymentsUseCase struct {
	create     deployments.CreateDeployment
	deployment deployments.Deployment
	events     []deployments.DeploymentEvent
	createErr  error
	findErr    error
	eventsErr  error
}

func (f *fakeDeploymentsUseCase) CreateDeployment(_ context.Context, req deployments.CreateDeployment) (deployments.Deployment, error) {
	f.create = req
	if f.createErr != nil {
		return deployments.Deployment{}, f.createErr
	}
	return f.deployment, nil
}

func (f *fakeDeploymentsUseCase) FindDeploymentByID(_ context.Context, _ string) (deployments.Deployment, error) {
	if f.findErr != nil {
		return deployments.Deployment{}, f.findErr
	}
	return f.deployment, nil
}

func (f *fakeDeploymentsUseCase) DeploymentEventsByDeploymentID(_ context.Context, _ string) ([]deployments.DeploymentEvent, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	return f.events, nil
}

func validDeployment() deployments.Deployment {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	risk := "customer-facing rollout"
	return deployments.Deployment{
		ID:           "dep_123",
		Service:      "payments",
		Environment:  "production",
		Version:      "v1.2.3",
		Status:       deployments.StatusPendingApproval,
		RequestedBy:  "alice",
		References:   []string{"https://github.com/org/repo/issues/123"},
		Risk:         &risk,
		RollbackPlan: "revert to v1.2.2",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func validEvent() deployments.DeploymentEvent {
	return deployments.DeploymentEvent{
		ID:           "evt_123",
		DeploymentID: "dep_123",
		Type:         deployments.EventDeploymentCreated,
		Actor:        "alice",
		CreatedAt:    time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
	}
}

func validCreateJSON() string {
	return `{
		"service":"payments",
		"environment":"production",
		"version":"v1.2.3",
		"requested_by":"alice",
		"references":["https://github.com/org/repo/issues/123"],
		"risk":"customer-facing rollout",
		"rollback_plan":"revert to v1.2.2"
	}`
}

func httpRequest(t *testing.T, handler http.Handler, method string, path string, body string) (*http.Response, string) {
	t.Helper()
	var reqBody *bytes.Reader
	if body == "" {
		reqBody = bytes.NewReader(nil)
	} else {
		reqBody = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, path, reqBody)
	if err != nil {
		t.Fatalf("NewRequest() error = %v, want nil", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder.Result(), recorder.Body.String()
}

func decodeJSON(t *testing.T, body string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), value); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v, want nil", body, err)
	}
}

func assertNotFoundBody(t *testing.T, body string) {
	t.Helper()
	var parsed map[string]string
	decodeJSON(t, body, &parsed)
	if parsed["error"] != "not_found" || parsed["message"] != "deployment not found" {
		t.Fatalf("not found body = %#v, want deployment not_found response", parsed)
	}
}
