// Package app coordinates deployment use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unionai/idd-sandbox/pkg/deployments"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("deployment not found")

// DeploymentRepository persists and retrieves deployments.
type DeploymentRepository interface {
	CreateDeployment(context.Context, deployments.Deployment, deployments.DeploymentEvent) error
	FindDeploymentByID(context.Context, string) (deployments.Deployment, error)
	DeploymentEventsByDeploymentID(context.Context, string) ([]deployments.DeploymentEvent, error)
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
func (a *App) CreateDeployment(ctx context.Context, req deployments.CreateDeployment) (deployments.Deployment, error) {
	now := a.now()
	deployment, err := deployments.NewDeployment(req, now)
	if err != nil {
		return deployments.Deployment{}, err
	}
	event, err := deployments.NewDeploymentCreatedEvent(deployment, now)
	if err != nil {
		return deployments.Deployment{}, err
	}
	if err := a.repository.CreateDeployment(ctx, deployment, event); err != nil {
		return deployments.Deployment{}, fmt.Errorf("create deployment: %w", err)
	}
	return deployment, nil
}

// FindDeploymentByID returns a deployment by ID.
func (a *App) FindDeploymentByID(ctx context.Context, id string) (deployments.Deployment, error) {
	return a.repository.FindDeploymentByID(ctx, id)
}

// DeploymentEventsByDeploymentID returns deployment events oldest first.
func (a *App) DeploymentEventsByDeploymentID(ctx context.Context, id string) ([]deployments.DeploymentEvent, error) {
	return a.repository.DeploymentEventsByDeploymentID(ctx, id)
}
