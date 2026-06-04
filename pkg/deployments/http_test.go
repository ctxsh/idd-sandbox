package deployments

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerPostDeploymentsSuccess(t *testing.T) {
	store := openTestStore(t)
	handler := NewHandler(store)

	resp, body := httpRequest(t, handler, http.MethodPost, "/deployments", validCreateJSON())

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusCreated)
	}
	var deployment Deployment
	decodeJSON(t, body, &deployment)
	if deployment.ID == "" {
		t.Fatal("ID is empty")
	}
	if deployment.Status != StatusPendingApproval {
		t.Fatalf("Status = %q, want %q", deployment.Status, StatusPendingApproval)
	}
	if deployment.Service != "payments" {
		t.Fatalf("Service = %q, want payments", deployment.Service)
	}
}

func TestHandlerPostDeploymentsInvalidCreate(t *testing.T) {
	store := openTestStore(t)
	handler := NewHandler(store)

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
	store := openTestStore(t)
	handler := NewHandler(store)

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
	store := openTestStore(t)
	handler := NewHandler(store)

	_, createBody := httpRequest(t, handler, http.MethodPost, "/deployments", validCreateJSON())
	var created Deployment
	decodeJSON(t, createBody, &created)

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments/"+created.ID, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var found Deployment
	decodeJSON(t, body, &found)
	if found.ID != created.ID {
		t.Fatalf("found ID = %q, want %q", found.ID, created.ID)
	}

	notFoundResp, notFoundBody := httpRequest(t, handler, http.MethodGet, "/deployments/missing", "")
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want %d", notFoundResp.StatusCode, notFoundBody, http.StatusNotFound)
	}
	assertNotFoundBody(t, notFoundBody)
}

func TestHandlerGetDeploymentEventsSuccessAndNotFound(t *testing.T) {
	store := openTestStore(t)
	handler := NewHandler(store)

	_, createBody := httpRequest(t, handler, http.MethodPost, "/deployments", validCreateJSON())
	var created Deployment
	decodeJSON(t, createBody, &created)

	resp, body := httpRequest(t, handler, http.MethodGet, "/deployments/"+created.ID+"/events", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
	var events []DeploymentEvent
	decodeJSON(t, body, &events)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Type != EventDeploymentCreated {
		t.Fatalf("event Type = %q, want %q", events[0].Type, EventDeploymentCreated)
	}

	notFoundResp, notFoundBody := httpRequest(t, handler, http.MethodGet, "/deployments/missing/events", "")
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want %d", notFoundResp.StatusCode, notFoundBody, http.StatusNotFound)
	}
	assertNotFoundBody(t, notFoundBody)
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
