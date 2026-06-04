// Package deployments owns the Deployment API domain contract and persistence.
package deployments

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Deployment statuses.
const (
	StatusPendingApproval = "pending_approval"
)

// Deployment event types.
const (
	EventDeploymentCreated = "deployment_created"
)

// ErrNotFound is returned when a deployment does not exist.
var ErrNotFound = errors.New("deployment not found")

// ValidationError reports field-specific validation failures.
type ValidationError struct {
	Fields map[string]string
}

// Error returns a stable validation error message.
func (e *ValidationError) Error() string {
	return "validation error"
}

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

// CreateDeployment is the client-provided payload for deployment creation.
type CreateDeployment struct {
	Service      string   `json:"service"`
	Environment  string   `json:"environment"`
	Version      string   `json:"version"`
	RequestedBy  string   `json:"requested_by"`
	References   []string `json:"references"`
	Risk         string   `json:"risk"`
	RollbackPlan string   `json:"rollback_plan"`
}

func newDeployment(req CreateDeployment, now time.Time) (Deployment, error) {
	req = normalizeCreate(req)
	if err := validateCreate(req); err != nil {
		return Deployment{}, err
	}

	id, err := newID("dep")
	if err != nil {
		return Deployment{}, fmt.Errorf("generate deployment id: %w", err)
	}

	var risk *string
	if req.Risk != "" {
		risk = &req.Risk
	}

	return Deployment{
		ID:           id,
		Service:      req.Service,
		Environment:  req.Environment,
		Version:      req.Version,
		Status:       StatusPendingApproval,
		RequestedBy:  req.RequestedBy,
		References:   req.References,
		Risk:         risk,
		RollbackPlan: req.RollbackPlan,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func newDeploymentCreatedEvent(deployment Deployment, now time.Time) (DeploymentEvent, error) {
	id, err := newID("evt")
	if err != nil {
		return DeploymentEvent{}, fmt.Errorf("generate deployment event id: %w", err)
	}

	return DeploymentEvent{
		ID:           id,
		DeploymentID: deployment.ID,
		Type:         EventDeploymentCreated,
		Actor:        deployment.RequestedBy,
		CreatedAt:    now,
	}, nil
}

func normalizeCreate(req CreateDeployment) CreateDeployment {
	req.Service = strings.TrimSpace(req.Service)
	req.Environment = strings.TrimSpace(req.Environment)
	req.Version = strings.TrimSpace(req.Version)
	req.RequestedBy = strings.TrimSpace(req.RequestedBy)
	req.Risk = strings.TrimSpace(req.Risk)
	req.RollbackPlan = strings.TrimSpace(req.RollbackPlan)
	for i, ref := range req.References {
		req.References[i] = strings.TrimSpace(ref)
	}
	return req
}

func validateCreate(req CreateDeployment) error {
	fields := map[string]string{}
	if req.Service == "" {
		fields["service"] = "is required"
	}
	if req.Environment == "" {
		fields["environment"] = "is required"
	}
	if req.Version == "" {
		fields["version"] = "is required"
	}
	if req.RequestedBy == "" {
		fields["requested_by"] = "is required"
	}
	if len(req.References) == 0 {
		fields["references"] = "must include at least one URL reference"
	} else {
		for _, ref := range req.References {
			if !isHTTPURL(ref) {
				fields["references"] = "must contain only valid HTTP or HTTPS URL references"
				break
			}
		}
	}
	if req.RollbackPlan == "" {
		fields["rollback_plan"] = "is required"
	}
	if req.Environment == "production" && req.Risk == "" {
		fields["risk"] = "is required for production deployments"
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func newID(prefix string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(buf[:]), nil
}
