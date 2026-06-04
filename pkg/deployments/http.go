package deployments

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// NewHandler returns an HTTP handler for the Deployment API.
func NewHandler(store *Store) http.Handler {
	return &handler{store: store}
}

type handler struct {
	store *Store
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
	var req CreateDeployment
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

	deployment, err := h.store.Create(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deployment)
}

func (h *handler) getDeploymentResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/deployments/")
	id, eventsPath, ok := parseDeploymentPath(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if eventsPath {
		events, err := h.store.EventsByDeploymentID(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, events)
		return
	}

	deployment, err := h.store.FindByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deployment)
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
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeValidationError(w, validationErr.Fields)
	case errors.Is(err, ErrNotFound):
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
