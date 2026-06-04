// Package deployments owns the Deployment domain contract and creation rules.
package deployments

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/unionai/idd-sandbox/pkg/types"
)

// Deployment statuses.
const (
	StatusPendingApproval = "pending_approval"
)

// Deployment event types.
const (
	EventDeploymentCreated = "deployment_created"
)

// ValidationError reports field-specific validation failures.
type ValidationError struct {
	Fields map[string]string
}

// Error returns a stable validation error message.
func (e *ValidationError) Error() string {
	return "validation error"
}

// NewDeployment validates creation input and returns a new pending Deployment.
func NewDeployment(req types.CreateDeployment, now time.Time) (types.Deployment, error) {
	req = normalizeCreate(req)
	if err := validateCreate(req); err != nil {
		return types.Deployment{}, err
	}

	id, err := newID("dep")
	if err != nil {
		return types.Deployment{}, fmt.Errorf("generate deployment id: %w", err)
	}

	var risk *string
	if req.Risk != "" {
		risk = &req.Risk
	}

	return types.Deployment{
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

// NewDeploymentCreatedEvent returns the initial event for a new Deployment.
func NewDeploymentCreatedEvent(deployment types.Deployment, now time.Time) (types.DeploymentEvent, error) {
	id, err := newID("evt")
	if err != nil {
		return types.DeploymentEvent{}, fmt.Errorf("generate deployment event id: %w", err)
	}

	return types.DeploymentEvent{
		ID:           id,
		DeploymentID: deployment.ID,
		Type:         EventDeploymentCreated,
		Actor:        deployment.RequestedBy,
		CreatedAt:    now,
	}, nil
}

func normalizeCreate(req types.CreateDeployment) types.CreateDeployment {
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

func validateCreate(req types.CreateDeployment) error {
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
