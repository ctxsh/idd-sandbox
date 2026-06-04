// Package types contains shared service data shapes.
package types

import "time"

// Deployment is the current state of a deployment request.
type Deployment struct {
	ID           string     `json:"id"`
	Service      string     `json:"service"`
	Environment  string     `json:"environment"`
	Version      string     `json:"version"`
	Status       string     `json:"status"`
	RequestedBy  string     `json:"requested_by"`
	ReviewedBy   *string    `json:"reviewed_by"`
	ReviewedAt   *time.Time `json:"reviewed_at"`
	References   []string   `json:"references"`
	Risk         *string    `json:"risk"`
	RollbackPlan string     `json:"rollback_plan"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	RolledBackAt *time.Time `json:"rolled_back_at"`
}

// DeploymentEvent is an append-only lifecycle history entry.
type DeploymentEvent struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Type         string    `json:"type"`
	Actor        string    `json:"actor"`
	Note         *string   `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateDeployment is the input for deployment creation.
type CreateDeployment struct {
	Service      string   `json:"service"`
	Environment  string   `json:"environment"`
	Version      string   `json:"version"`
	RequestedBy  string   `json:"requested_by"`
	References   []string `json:"references"`
	Risk         string   `json:"risk"`
	RollbackPlan string   `json:"rollback_plan"`
}

// ListDeploymentsRequest filters and paginates deployment list results.
type ListDeploymentsRequest struct {
	Service     string
	Environment string
	Status      string
	Limit       int
	Offset      int
}

// ListDeploymentsResponse is a page of deployment current-state records.
type ListDeploymentsResponse struct {
	Items      []Deployment `json:"items"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	NextOffset *int         `json:"next_offset"`
}
