package app

import (
	"context"
	"errors"
	"testing"

	"github.com/unionai/idd-sandbox/pkg/deployments"
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

	_, err := application.CreateDeployment(context.Background(), deployments.CreateDeployment{})
	var validationErr *deployments.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateDeployment() error = %v, want ValidationError", err)
	}
	if repository.createCalled {
		t.Fatal("repository CreateDeployment called for invalid request")
	}
}

type fakeRepository struct {
	createCalled bool
	deployment   deployments.Deployment
	event        deployments.DeploymentEvent
}

func (f *fakeRepository) CreateDeployment(_ context.Context, deployment deployments.Deployment, event deployments.DeploymentEvent) error {
	f.createCalled = true
	f.deployment = deployment
	f.event = event
	return nil
}

func (f *fakeRepository) FindDeploymentByID(_ context.Context, _ string) (deployments.Deployment, error) {
	return deployments.Deployment{}, nil
}

func (f *fakeRepository) DeploymentEventsByDeploymentID(_ context.Context, _ string) ([]deployments.DeploymentEvent, error) {
	return nil, nil
}

func validCreateDeployment() deployments.CreateDeployment {
	return deployments.CreateDeployment{
		Service:      "payments",
		Environment:  "production",
		Version:      "v1.2.3",
		RequestedBy:  "alice",
		References:   []string{"https://github.com/org/repo/issues/123"},
		Risk:         "customer-facing rollout",
		RollbackPlan: "revert to v1.2.2",
	}
}
