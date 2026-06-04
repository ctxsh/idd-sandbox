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
	"github.com/unionai/idd-sandbox/pkg/types"
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
	var deployment types.Deployment
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

func TestHandlerGetDeploymentsSuccess(t *testing.T) {
	nextOffset := 3
	useCase := &fakeDeploymentsUseCase{
		listResponse: types.ListDeploymentsResponse{
			Items:      []types.Deployment{validDeployment()},
			Limit:      2,
			Offset:     1,
			NextOffset: &nextOffset,
		},
	}
	handler := NewHandler(useCase)

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments?service=%20payments%20&environment=production&status=pending_approval&limit=2&offset=1", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var parsed types.ListDeploymentsResponse
	decodeJSON(t, body, &parsed)
	if len(parsed.Items) != 1 || parsed.Items[0].ID != "dep_123" {
		t.Fatalf("items = %#v, want dep_123", parsed.Items)
	}
	if parsed.Limit != 2 || parsed.Offset != 1 || parsed.NextOffset == nil || *parsed.NextOffset != 3 {
		t.Fatalf("pagination = limit %d offset %d next %v, want 2/1/3", parsed.Limit, parsed.Offset, parsed.NextOffset)
	}
	if useCase.list.Service != "payments" {
		t.Fatalf("list Service = %q, want payments", useCase.list.Service)
	}
	if useCase.list.Environment != "production" {
		t.Fatalf("list Environment = %q, want production", useCase.list.Environment)
	}
	if useCase.list.Status != deployments.StatusPendingApproval {
		t.Fatalf("list Status = %q, want %q", useCase.list.Status, deployments.StatusPendingApproval)
	}
	if useCase.list.Limit != 2 || useCase.list.Offset != 1 {
		t.Fatalf("list pagination = limit %d offset %d, want 2/1", useCase.list.Limit, useCase.list.Offset)
	}
}

func TestHandlerGetDeploymentsDefaultsPagination(t *testing.T) {
	useCase := &fakeDeploymentsUseCase{
		listResponse: types.ListDeploymentsResponse{Items: []types.Deployment{}, Limit: 50},
	}
	handler := NewHandler(useCase)

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	if useCase.list.Limit != 50 {
		t.Fatalf("list Limit = %d, want default 50", useCase.list.Limit)
	}
	if useCase.list.Offset != 0 {
		t.Fatalf("list Offset = %d, want default 0", useCase.list.Offset)
	}
}

func TestHandlerGetDeploymentsRejectsInvalidQuery(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		field string
	}{
		{name: "unsupported", path: "/deployments?version=v1", field: "version"},
		{name: "duplicate", path: "/deployments?service=payments&service=billing", field: "service"},
		{name: "empty service", path: "/deployments?service=%20", field: "service"},
		{name: "invalid status", path: "/deployments?status=waiting", field: "status"},
		{name: "malformed limit", path: "/deployments?limit=abc", field: "limit"},
		{name: "empty limit", path: "/deployments?limit=", field: "limit"},
		{name: "zero limit", path: "/deployments?limit=0", field: "limit"},
		{name: "limit too high", path: "/deployments?limit=101", field: "limit"},
		{name: "negative offset", path: "/deployments?offset=-1", field: "offset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useCase := &fakeDeploymentsUseCase{}
			handler := NewHandler(useCase)

			resp, body := httpRequest(t, handler, http.MethodGet, tt.path, "")

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
			if parsed.Fields[tt.field] == "" {
				t.Fatalf("field %q validation missing in %#v", tt.field, parsed.Fields)
			}
			if useCase.listCalled {
				t.Fatal("ListDeployments called for invalid query")
			}
		})
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
	var found types.Deployment
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
		events: []types.DeploymentEvent{validEvent()},
	})

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments/dep_123/events", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var events []types.DeploymentEvent
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

func TestHandlerDeploymentsMethodNotAllowed(t *testing.T) {
	handler := NewHandler(&fakeDeploymentsUseCase{})

	resp, body := httpRequest(t, handler, http.MethodPut, "/deployments", "")

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusMethodNotAllowed)
	}
	if resp.Header.Get("Allow") != "GET, POST" {
		t.Fatalf("Allow = %q, want GET, POST", resp.Header.Get("Allow"))
	}
}

type fakeDeploymentsUseCase struct {
	create       types.CreateDeployment
	list         types.ListDeploymentsRequest
	deployment   types.Deployment
	listResponse types.ListDeploymentsResponse
	events       []types.DeploymentEvent
	createErr    error
	listErr      error
	findErr      error
	eventsErr    error
	listCalled   bool
}

func (f *fakeDeploymentsUseCase) CreateDeployment(_ context.Context, req types.CreateDeployment) (types.Deployment, error) {
	f.create = req
	if f.createErr != nil {
		return types.Deployment{}, f.createErr
	}
	return f.deployment, nil
}

func (f *fakeDeploymentsUseCase) ListDeployments(_ context.Context, req types.ListDeploymentsRequest) (types.ListDeploymentsResponse, error) {
	f.listCalled = true
	f.list = req
	if f.listErr != nil {
		return types.ListDeploymentsResponse{}, f.listErr
	}
	return f.listResponse, nil
}

func (f *fakeDeploymentsUseCase) FindDeploymentByID(_ context.Context, _ string) (types.Deployment, error) {
	if f.findErr != nil {
		return types.Deployment{}, f.findErr
	}
	return f.deployment, nil
}

func (f *fakeDeploymentsUseCase) DeploymentEventsByDeploymentID(_ context.Context, _ string) ([]types.DeploymentEvent, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	return f.events, nil
}

func validDeployment() types.Deployment {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	risk := "customer-facing rollout"
	return types.Deployment{
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

func validEvent() types.DeploymentEvent {
	return types.DeploymentEvent{
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
