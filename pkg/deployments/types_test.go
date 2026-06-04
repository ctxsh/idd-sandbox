package deployments

import (
	"errors"
	"testing"
	"time"

	"github.com/unionai/idd-sandbox/pkg/types"
)

func TestNewDeploymentProductionRequiresRisk(t *testing.T) {
	_, err := NewDeployment(validCreateDeployment(), time.Now())
	if err != nil {
		t.Fatalf("newDeployment() error = %v, want nil", err)
	}

	req := validCreateDeployment()
	req.Risk = ""
	_, err = NewDeployment(req, time.Now())
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("newDeployment() error = %v, want ValidationError", err)
	}
	if got := validationErr.Fields["risk"]; got != "is required for production deployments" {
		t.Fatalf("risk validation = %q, want production risk requirement", got)
	}
}

func TestNewDeploymentNonProductionAllowsOmittedRisk(t *testing.T) {
	req := validCreateDeployment()
	req.Environment = "staging"
	req.Risk = ""

	deployment, err := NewDeployment(req, time.Now())
	if err != nil {
		t.Fatalf("newDeployment() error = %v, want nil", err)
	}
	if deployment.Risk != nil {
		t.Fatalf("Risk = %v, want nil", *deployment.Risk)
	}
}

func TestNewDeploymentRequiredFields(t *testing.T) {
	req := types.CreateDeployment{}

	_, err := NewDeployment(req, time.Now())
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("newDeployment() error = %v, want ValidationError", err)
	}

	wantFields := []string{"service", "environment", "version", "requested_by", "references", "rollback_plan"}
	for _, field := range wantFields {
		if validationErr.Fields[field] == "" {
			t.Fatalf("validation field %q missing in %#v", field, validationErr.Fields)
		}
	}
}

func TestNewDeploymentInvalidReferences(t *testing.T) {
	tests := []struct {
		name       string
		references []string
	}{
		{name: "empty", references: []string{""}},
		{name: "not url", references: []string{"not-a-url"}},
		{name: "missing host", references: []string{"https://"}},
		{name: "unsupported scheme", references: []string{"ftp://example.com/plan"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCreateDeployment()
			req.References = tt.references

			_, err := NewDeployment(req, time.Now())
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("newDeployment() error = %v, want ValidationError", err)
			}
			if validationErr.Fields["references"] == "" {
				t.Fatalf("references validation missing in %#v", validationErr.Fields)
			}
		})
	}
}

func TestNewDeploymentGeneratesCurrentState(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	deployment, err := NewDeployment(validCreateDeployment(), now)
	if err != nil {
		t.Fatalf("newDeployment() error = %v, want nil", err)
	}

	if deployment.ID == "" {
		t.Fatal("ID is empty")
	}
	if deployment.Status != StatusPendingApproval {
		t.Fatalf("Status = %q, want %q", deployment.Status, StatusPendingApproval)
	}
	if !deployment.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", deployment.CreatedAt, now)
	}
	if !deployment.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", deployment.UpdatedAt, now)
	}
}

func TestNormalizeListDeploymentsDefaultsAndTrims(t *testing.T) {
	req, err := NormalizeListDeployments(types.ListDeploymentsRequest{
		Service:     " payments ",
		Environment: " production ",
		Status:      " pending_approval ",
	})
	if err != nil {
		t.Fatalf("NormalizeListDeployments() error = %v, want nil", err)
	}
	if req.Service != "payments" {
		t.Fatalf("Service = %q, want payments", req.Service)
	}
	if req.Environment != "production" {
		t.Fatalf("Environment = %q, want production", req.Environment)
	}
	if req.Status != StatusPendingApproval {
		t.Fatalf("Status = %q, want %q", req.Status, StatusPendingApproval)
	}
	if req.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", req.Limit)
	}
	if req.Offset != 0 {
		t.Fatalf("Offset = %d, want 0", req.Offset)
	}
}

func TestNormalizeListDeploymentsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		req   types.ListDeploymentsRequest
		field string
	}{
		{name: "empty service", req: types.ListDeploymentsRequest{Service: " "}, field: "service"},
		{name: "empty environment", req: types.ListDeploymentsRequest{Environment: " "}, field: "environment"},
		{name: "empty status", req: types.ListDeploymentsRequest{Status: " "}, field: "status"},
		{name: "invalid status", req: types.ListDeploymentsRequest{Status: "waiting"}, field: "status"},
		{name: "negative limit", req: types.ListDeploymentsRequest{Limit: -1}, field: "limit"},
		{name: "limit too high", req: types.ListDeploymentsRequest{Limit: 101}, field: "limit"},
		{name: "negative offset", req: types.ListDeploymentsRequest{Offset: -1}, field: "offset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeListDeployments(tt.req)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("NormalizeListDeployments() error = %v, want ValidationError", err)
			}
			if validationErr.Fields[tt.field] == "" {
				t.Fatalf("validation field %q missing in %#v", tt.field, validationErr.Fields)
			}
		})
	}
}

func validCreateDeployment() types.CreateDeployment {
	return types.CreateDeployment{
		Service:      "payments",
		Environment:  "production",
		Version:      "v1.2.3",
		RequestedBy:  "alice",
		References:   []string{"https://github.com/org/repo/issues/123"},
		Risk:         "customer-facing rollout",
		RollbackPlan: "revert to v1.2.2",
	}
}
