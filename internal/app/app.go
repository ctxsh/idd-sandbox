// Package app coordinates deployment use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unionai/idd-sandbox/pkg/deployments"
	"github.com/unionai/idd-sandbox/pkg/types"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("deployment not found")

// DeploymentRepository persists and retrieves deployments.
type DeploymentRepository interface {
	CreateDeployment(context.Context, types.Deployment, types.DeploymentEvent) error
	ListDeployments(context.Context, types.ListDeploymentsRequest) (types.ListDeploymentsResponse, error)
	FindDeploymentByID(context.Context, string) (types.Deployment, error)
	DeploymentEventsByDeploymentID(context.Context, string) ([]types.DeploymentEvent, error)
}

// App coordinates domain rules with persistence.
type App struct {
	repository DeploymentRepository
	now        func() time.Time
}

// New returns an application coordinator.
func New(repository DeploymentRepository) *App {
	return &App{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// CreateDeployment creates and persists a deployment with its initial event.
func (a *App) CreateDeployment(ctx context.Context, req types.CreateDeployment) (types.Deployment, error) {
	now := a.now()
	deployment, err := deployments.NewDeployment(req, now)
	if err != nil {
		return types.Deployment{}, err
	}
	event, err := deployments.NewDeploymentCreatedEvent(deployment, now)
	if err != nil {
		return types.Deployment{}, err
	}
	if err := a.repository.CreateDeployment(ctx, deployment, event); err != nil {
		return types.Deployment{}, fmt.Errorf("create deployment: %w", err)
	}
	return deployment, nil
}

// ListDeployments returns deployment current-state records matching the request.
func (a *App) ListDeployments(ctx context.Context, req types.ListDeploymentsRequest) (types.ListDeploymentsResponse, error) {
	normalized, err := deployments.NormalizeListDeployments(req)
	if err != nil {
		return types.ListDeploymentsResponse{}, err
	}
	return a.repository.ListDeployments(ctx, normalized)
}

// FindDeploymentByID returns a deployment by ID.
func (a *App) FindDeploymentByID(ctx context.Context, id string) (types.Deployment, error) {
	return a.repository.FindDeploymentByID(ctx, id)
}

// DeploymentEventsByDeploymentID returns deployment events oldest first.
func (a *App) DeploymentEventsByDeploymentID(ctx context.Context, id string) ([]types.DeploymentEvent, error) {
	return a.repository.DeploymentEventsByDeploymentID(ctx, id)
}
