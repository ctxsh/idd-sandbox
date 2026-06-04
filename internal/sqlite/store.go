// Package sqlite provides SQLite persistence adapters.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unionai/idd-sandbox/internal/app"
	"github.com/unionai/idd-sandbox/pkg/types"

	// Register the pure-Go SQLite driver used by Open.
	_ "modernc.org/sqlite"
)

// Store persists records in SQLite.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite store and initializes its schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("initialize sqlite store: %w; close database: %v", err, closeErr)
		}
		return nil, fmt.Errorf("initialize sqlite store: %w", err)
	}
	return store, nil
}

// Close closes the underlying SQLite database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS deployments (
	id TEXT PRIMARY KEY,
	service TEXT NOT NULL,
	environment TEXT NOT NULL,
	version TEXT NOT NULL,
	status TEXT NOT NULL,
	requested_by TEXT NOT NULL,
	reviewed_by TEXT,
	reviewed_at TEXT,
	references_json TEXT NOT NULL,
	risk TEXT,
	rollback_plan TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	started_at TEXT,
	completed_at TEXT,
	rolled_back_at TEXT
);

CREATE TABLE IF NOT EXISTS deployment_events (
	id TEXT PRIMARY KEY,
	deployment_id TEXT NOT NULL,
	type TEXT NOT NULL,
	actor TEXT NOT NULL,
	note TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS deployment_events_deployment_id_created_at_idx
	ON deployment_events (deployment_id, created_at, id);

CREATE INDEX IF NOT EXISTS deployments_created_at_id_idx
	ON deployments (created_at DESC, id DESC);
`)
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// CreateDeployment persists a deployment and event in one transaction.
func (s *Store) CreateDeployment(ctx context.Context, deployment types.Deployment, event types.DeploymentEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create deployment transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := insertDeployment(ctx, tx, deployment); err != nil {
		return err
	}
	if err := insertDeploymentEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create deployment transaction: %w", err)
	}
	committed = true

	return nil
}

// ListDeployments returns deployment current-state records newest first.
func (s *Store) ListDeployments(ctx context.Context, req types.ListDeploymentsRequest) (types.ListDeploymentsResponse, error) {
	query := deploymentSelectQuery()
	var predicates []string
	var args []any
	if req.Service != "" {
		predicates = append(predicates, "service = ?")
		args = append(args, req.Service)
	}
	if req.Environment != "" {
		predicates = append(predicates, "environment = ?")
		args = append(args, req.Environment)
	}
	if req.Status != "" {
		predicates = append(predicates, "status = ?")
		args = append(args, req.Status)
	}
	if len(predicates) > 0 {
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, req.Limit, req.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return types.ListDeploymentsResponse{}, fmt.Errorf("query deployments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	items := []types.Deployment{}
	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return types.ListDeploymentsResponse{}, err
		}
		items = append(items, deployment)
	}
	if err := rows.Err(); err != nil {
		return types.ListDeploymentsResponse{}, fmt.Errorf("iterate deployments: %w", err)
	}

	var nextOffset *int
	if len(items) == req.Limit {
		next := req.Offset + req.Limit
		nextOffset = &next
	}
	return types.ListDeploymentsResponse{
		Items:      items,
		Limit:      req.Limit,
		Offset:     req.Offset,
		NextOffset: nextOffset,
	}, nil
}

// FindDeploymentByID returns a deployment by ID.
func (s *Store) FindDeploymentByID(ctx context.Context, id string) (types.Deployment, error) {
	row := s.db.QueryRowContext(ctx, deploymentSelectQuery()+` WHERE id = ?`, id)
	deployment, err := scanDeployment(row)
	if err != nil {
		return types.Deployment{}, err
	}
	return deployment, nil
}

// DeploymentEventsByDeploymentID returns deployment events oldest first.
func (s *Store) DeploymentEventsByDeploymentID(ctx context.Context, deploymentID string) ([]types.DeploymentEvent, error) {
	if _, err := s.FindDeploymentByID(ctx, deploymentID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, deployment_id, type, actor, note, created_at
FROM deployment_events
WHERE deployment_id = ?
ORDER BY created_at ASC, id ASC
`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment events: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var events []types.DeploymentEvent
	for rows.Next() {
		event, err := scanDeploymentEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment events: %w", err)
	}
	return events, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func insertDeployment(ctx context.Context, tx *sql.Tx, deployment types.Deployment) error {
	referencesJSON, err := json.Marshal(deployment.References)
	if err != nil {
		return fmt.Errorf("marshal deployment references: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO deployments (
	id, service, environment, version, status, requested_by, reviewed_by, reviewed_at,
	references_json, risk, rollback_plan, created_at, updated_at, started_at, completed_at, rolled_back_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		deployment.ID,
		deployment.Service,
		deployment.Environment,
		deployment.Version,
		deployment.Status,
		deployment.RequestedBy,
		nullableString(deployment.ReviewedBy),
		nullableTime(deployment.ReviewedAt),
		string(referencesJSON),
		nullableString(deployment.Risk),
		deployment.RollbackPlan,
		formatTime(deployment.CreatedAt),
		formatTime(deployment.UpdatedAt),
		nullableTime(deployment.StartedAt),
		nullableTime(deployment.CompletedAt),
		nullableTime(deployment.RolledBackAt),
	)
	if err != nil {
		return fmt.Errorf("insert deployment: %w", err)
	}
	return nil
}

func insertDeploymentEvent(ctx context.Context, tx *sql.Tx, event types.DeploymentEvent) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO deployment_events (id, deployment_id, type, actor, note, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`,
		event.ID,
		event.DeploymentID,
		event.Type,
		event.Actor,
		nullableString(event.Note),
		formatTime(event.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert deployment event: %w", err)
	}
	return nil
}

func deploymentSelectQuery() string {
	return `
SELECT id, service, environment, version, status, requested_by, reviewed_by, reviewed_at,
	references_json, risk, rollback_plan, created_at, updated_at, started_at, completed_at, rolled_back_at
FROM deployments`
}

func scanDeployment(row rowScanner) (types.Deployment, error) {
	var deployment types.Deployment
	var reviewedBy sql.NullString
	var reviewedAt sql.NullString
	var referencesJSON string
	var risk sql.NullString
	var startedAt sql.NullString
	var completedAt sql.NullString
	var rolledBackAt sql.NullString
	var createdAt string
	var updatedAt string

	err := row.Scan(
		&deployment.ID,
		&deployment.Service,
		&deployment.Environment,
		&deployment.Version,
		&deployment.Status,
		&deployment.RequestedBy,
		&reviewedBy,
		&reviewedAt,
		&referencesJSON,
		&risk,
		&deployment.RollbackPlan,
		&createdAt,
		&updatedAt,
		&startedAt,
		&completedAt,
		&rolledBackAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Deployment{}, app.ErrNotFound
	}
	if err != nil {
		return types.Deployment{}, fmt.Errorf("scan deployment: %w", err)
	}

	if err := json.Unmarshal([]byte(referencesJSON), &deployment.References); err != nil {
		return types.Deployment{}, fmt.Errorf("unmarshal deployment references: %w", err)
	}
	deployment.ReviewedBy = stringPtrFromNull(reviewedBy)
	deployment.Risk = stringPtrFromNull(risk)

	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment created_at: %w", err)
	}
	deployment.CreatedAt = parsedCreatedAt
	parsedUpdatedAt, err := parseTime(updatedAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment updated_at: %w", err)
	}
	deployment.UpdatedAt = parsedUpdatedAt

	deployment.ReviewedAt, err = timePtrFromNull(reviewedAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment reviewed_at: %w", err)
	}
	deployment.StartedAt, err = timePtrFromNull(startedAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment started_at: %w", err)
	}
	deployment.CompletedAt, err = timePtrFromNull(completedAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment completed_at: %w", err)
	}
	deployment.RolledBackAt, err = timePtrFromNull(rolledBackAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("parse deployment rolled_back_at: %w", err)
	}

	return deployment, nil
}

func scanDeploymentEvent(row rowScanner) (types.DeploymentEvent, error) {
	var event types.DeploymentEvent
	var note sql.NullString
	var createdAt string

	err := row.Scan(
		&event.ID,
		&event.DeploymentID,
		&event.Type,
		&event.Actor,
		&note,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.DeploymentEvent{}, app.ErrNotFound
	}
	if err != nil {
		return types.DeploymentEvent{}, fmt.Errorf("scan deployment event: %w", err)
	}
	event.Note = stringPtrFromNull(note)
	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return types.DeploymentEvent{}, fmt.Errorf("parse deployment event created_at: %w", err)
	}
	event.CreatedAt = parsedCreatedAt
	return event, nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func stringPtrFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePtrFromNull(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
