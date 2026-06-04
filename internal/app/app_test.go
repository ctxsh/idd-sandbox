package app

import (
	"context"
	"errors"
	"testing"

	"github.com/unionai/idd-sandbox/pkg/deployments"
	"github.com/unionai/idd-sandbox/pkg/types"
)

func TestAppCreateDeploymentBuildsDomainRecordsAndPersistsThem(t *testing.T) {
	repository := &fakeRepository{}
	application := New(repository)

	deployment, err := application.CreateDeployment(context.Background(), validCreateDeployment())
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	if deployment.ID == "" {
		t.Fatal("deployment ID is empty")
	}
	if deployment.Status != deployments.StatusPendingApproval {
		t.Fatalf("deployment Status = %q, want %q", deployment.Status, deployments.StatusPendingApproval)
	}
	if repository.deployment.ID != deployment.ID {
		t.Fatalf("persisted deployment ID = %q, want %q", repository.deployment.ID, deployment.ID)
	}
	if repository.event.DeploymentID != deployment.ID {
		t.Fatalf("event DeploymentID = %q, want %q", repository.event.DeploymentID, deployment.ID)
	}
	if repository.event.Type != deployments.EventDeploymentCreated {
		t.Fatalf("event Type = %q, want %q", repository.event.Type, deployments.EventDeploymentCreated)
	}
}

func TestAppCreateDeploymentReturnsValidationErrorsBeforePersistence(t *testing.T) {
	repository := &fakeRepository{}
	application := New(repository)

	_, err := application.CreateDeployment(context.Background(), types.CreateDeployment{})
	var validationErr *deployments.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateDeployment() error = %v, want ValidationError", err)
	}
	if repository.createCalled {
		t.Fatal("repository CreateDeployment called for invalid request")
	}
}

func TestAppListDeploymentsNormalizesRequestBeforeRepository(t *testing.T) {
	repository := &fakeRepository{
		listResponse: types.ListDeploymentsResponse{Limit: 50},
	}
	application := New(repository)

	_, err := application.ListDeployments(context.Background(), types.ListDeploymentsRequest{
		Service:     " payments ",
		Environment: " production ",
		Status:      " pending_approval ",
	})
	if err != nil {
		t.Fatalf("ListDeployments() error = %v, want nil", err)
	}

	if !repository.listCalled {
		t.Fatal("repository ListDeployments was not called")
	}
	if repository.list.Service != "payments" {
		t.Fatalf("list Service = %q, want payments", repository.list.Service)
	}
	if repository.list.Environment != "production" {
		t.Fatalf("list Environment = %q, want production", repository.list.Environment)
	}
	if repository.list.Status != deployments.StatusPendingApproval {
		t.Fatalf("list Status = %q, want %q", repository.list.Status, deployments.StatusPendingApproval)
	}
	if repository.list.Limit != 50 {
		t.Fatalf("list Limit = %d, want default 50", repository.list.Limit)
	}
	if repository.list.Offset != 0 {
		t.Fatalf("list Offset = %d, want default 0", repository.list.Offset)
	}
}

func TestAppListDeploymentsReturnsValidationErrorsBeforeRepository(t *testing.T) {
	repository := &fakeRepository{}
	application := New(repository)

	_, err := application.ListDeployments(context.Background(), types.ListDeploymentsRequest{Status: "waiting"})
	var validationErr *deployments.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ListDeployments() error = %v, want ValidationError", err)
	}
	if repository.listCalled {
		t.Fatal("repository ListDeployments called for invalid request")
	}
}

type fakeRepository struct {
	createCalled bool
	listCalled   bool
	deployment   types.Deployment
	event        types.DeploymentEvent
	list         types.ListDeploymentsRequest
	listResponse types.ListDeploymentsResponse
}

func (f *fakeRepository) CreateDeployment(_ context.Context, deployment types.Deployment, event types.DeploymentEvent) error {
	f.createCalled = true
	f.deployment = deployment
	f.event = event
	return nil
}

func (f *fakeRepository) ListDeployments(_ context.Context, req types.ListDeploymentsRequest) (types.ListDeploymentsResponse, error) {
	f.listCalled = true
	f.list = req
	return f.listResponse, nil
}

func (f *fakeRepository) FindDeploymentByID(_ context.Context, _ string) (types.Deployment, error) {
	return types.Deployment{}, nil
}

func (f *fakeRepository) DeploymentEventsByDeploymentID(_ context.Context, _ string) ([]types.DeploymentEvent, error) {
	return nil, nil
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
