// Package server provides HTTP transport for the service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/unionai/idd-sandbox/internal/app"
	"github.com/unionai/idd-sandbox/pkg/deployments"
)

type deploymentsUseCase interface {
	CreateDeployment(context.Context, deployments.CreateDeployment) (deployments.Deployment, error)
	FindDeploymentByID(context.Context, string) (deployments.Deployment, error)
	DeploymentEventsByDeploymentID(context.Context, string) ([]deployments.DeploymentEvent, error)
}

// NewHandler returns an HTTP handler for the service.
func NewHandler(deployments deploymentsUseCase) http.Handler {
	return &handler{deployments: deployments}
}

type handler struct {
	deployments deploymentsUseCase
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/deployments" && r.Method == http.MethodPost:
		h.createDeployment(w, r)
	case strings.HasPrefix(r.URL.Path, "/deployments/") && r.Method == http.MethodGet:
		h.getDeploymentResource(w, r)
	case r.URL.Path == "/deployments":
		methodNotAllowed(w)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) createDeployment(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Printf("close request body: %v", err)
		}
	}()
	var req createDeploymentRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeValidationError(w, map[string]string{"body": decodeErrorMessage(err)})
		return
	}
	if decoder.Decode(&struct{}{}) == nil {
		writeValidationError(w, map[string]string{"body": "must contain a single JSON object"})
		return
	}

	deployment, err := h.deployments.CreateDeployment(r.Context(), req.toDomain())
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deploymentResponseFromDomain(deployment))
}

func (h *handler) getDeploymentResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/deployments/")
	id, eventsPath, ok := parseDeploymentPath(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if eventsPath {
		events, err := h.deployments.DeploymentEventsByDeploymentID(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, eventResponsesFromDomain(events))
		return
	}

	deployment, err := h.deployments.FindDeploymentByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deploymentResponseFromDomain(deployment))
}

func parseDeploymentPath(path string) (id string, eventsPath bool, ok bool) {
	if path == "" {
		return "", false, false
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], false, true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "events" {
		return parts[0], true, true
	}
	return "", false, false
}

func handleError(w http.ResponseWriter, err error) {
	var validationErr *deployments.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeValidationError(w, validationErr.Fields)
	case errors.Is(err, app.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":   "not_found",
			"message": "deployment not found",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "internal_error",
			"message": "internal server error",
		})
	}
}

func writeValidationError(w http.ResponseWriter, fields map[string]string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  "validation_error",
		"fields": fields,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", http.MethodPost)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error":   "method_not_allowed",
		"message": "method not allowed",
	})
}

func decodeErrorMessage(err error) string {
	const unknownFieldPrefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), unknownFieldPrefix) {
		field := strings.Trim(err.Error()[len(unknownFieldPrefix):], `"`)
		return fmt.Sprintf("unknown field %q", field)
	}
	return "invalid JSON request body"
}

type createDeploymentRequest struct {
	Service      string   `json:"service"`
	Environment  string   `json:"environment"`
	Version      string   `json:"version"`
	RequestedBy  string   `json:"requested_by"`
	References   []string `json:"references"`
	Risk         string   `json:"risk"`
	RollbackPlan string   `json:"rollback_plan"`
}

func (r createDeploymentRequest) toDomain() deployments.CreateDeployment {
	return deployments.CreateDeployment{
		Service:      r.Service,
		Environment:  r.Environment,
		Version:      r.Version,
		RequestedBy:  r.RequestedBy,
		References:   r.References,
		Risk:         r.Risk,
		RollbackPlan: r.RollbackPlan,
	}
}

type deploymentResponse struct {
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

func deploymentResponseFromDomain(deployment deployments.Deployment) deploymentResponse {
	return deploymentResponse{
		ID:           deployment.ID,
		Service:      deployment.Service,
		Environment:  deployment.Environment,
		Version:      deployment.Version,
		Status:       deployment.Status,
		RequestedBy:  deployment.RequestedBy,
		ReviewedBy:   deployment.ReviewedBy,
		ReviewedAt:   deployment.ReviewedAt,
		References:   deployment.References,
		Risk:         deployment.Risk,
		RollbackPlan: deployment.RollbackPlan,
		CreatedAt:    deployment.CreatedAt,
		UpdatedAt:    deployment.UpdatedAt,
		StartedAt:    deployment.StartedAt,
		CompletedAt:  deployment.CompletedAt,
		RolledBackAt: deployment.RolledBackAt,
	}
}

type eventResponse struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Type         string    `json:"type"`
	Actor        string    `json:"actor"`
	Note         *string   `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

func eventResponsesFromDomain(events []deployments.DeploymentEvent) []eventResponse {
	responses := make([]eventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, eventResponse{
			ID:           event.ID,
			DeploymentID: event.DeploymentID,
			Type:         event.Type,
			Actor:        event.Actor,
			Note:         event.Note,
			CreatedAt:    event.CreatedAt,
		})
	}
	return responses
}
