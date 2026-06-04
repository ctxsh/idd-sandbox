package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/unionai/idd-sandbox/internal/app"
	"github.com/unionai/idd-sandbox/pkg/deployments"
	"github.com/unionai/idd-sandbox/pkg/types"
)

func TestStoreCreateDeploymentPersistsDeploymentAndInitialEvent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	deployment, event := newTestDeploymentAndEvent(t)

	if err := store.CreateDeployment(ctx, deployment, event); err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	found, err := store.FindDeploymentByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("FindDeploymentByID() error = %v, want nil", err)
	}
	if found.ID != deployment.ID {
		t.Fatalf("found ID = %q, want %q", found.ID, deployment.ID)
	}
	if len(found.References) != 1 || found.References[0] != "https://github.com/org/repo/issues/123" {
		t.Fatalf("References = %#v, want original reference", found.References)
	}

	events, err := store.DeploymentEventsByDeploymentID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("DeploymentEventsByDeploymentID() error = %v, want nil", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ID != event.ID {
		t.Fatalf("event ID = %q, want %q", events[0].ID, event.ID)
	}
}

func TestStoreReopenFindsDeployment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "records.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	deployment, event := newTestDeploymentAndEvent(t)
	if err := store.CreateDeployment(ctx, deployment, event); err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after reopen error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	})

	found, err := reopened.FindDeploymentByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("FindDeploymentByID() error = %v, want nil", err)
	}
	if found.ID != deployment.ID {
		t.Fatalf("found ID = %q, want %q", found.ID, deployment.ID)
	}
}

func TestStoreDeploymentEventsByDeploymentIDOldestFirst(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	deployment, event := newTestDeploymentAndEvent(t)
	if err := store.CreateDeployment(ctx, deployment, event); err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	later := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	_, err := store.db.ExecContext(ctx, `
INSERT INTO deployment_events (id, deployment_id, type, actor, note, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, "evt_later", deployment.ID, "deployment_started", "alice", nil, later)
	if err != nil {
		t.Fatalf("insert later event: %v", err)
	}

	events, err := store.DeploymentEventsByDeploymentID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("DeploymentEventsByDeploymentID() error = %v, want nil", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].Type != deployments.EventDeploymentCreated || events[1].Type != "deployment_started" {
		t.Fatalf("event order = [%q, %q], want created then started", events[0].Type, events[1].Type)
	}
}

func TestStoreMissingIDsReturnNotFound(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.FindDeploymentByID(ctx, "missing"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("FindDeploymentByID() error = %v, want ErrNotFound", err)
	}
	if _, err := store.DeploymentEventsByDeploymentID(ctx, "missing"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("DeploymentEventsByDeploymentID() error = %v, want ErrNotFound", err)
	}
}

func TestStoreCreateDeploymentRollsBackWhenEventInsertFails(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `DROP TABLE deployment_events`)
	if err != nil {
		t.Fatalf("drop deployment_events: %v", err)
	}
	deployment, event := newTestDeploymentAndEvent(t)

	err = store.CreateDeployment(ctx, deployment, event)
	if err == nil {
		t.Fatal("CreateDeployment() error = nil, want event insert failure")
	}

	var count int
	err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments`).Scan(&count)
	if err != nil {
		t.Fatalf("count deployments: %v", err)
	}
	if count != 0 {
		t.Fatalf("deployment rows after failed CreateDeployment = %d, want 0", count)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "records.db"))
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	})
	return store
}

func newTestDeploymentAndEvent(t *testing.T) (types.Deployment, types.DeploymentEvent) {
	t.Helper()
	now := time.Now().UTC()
	deployment, err := deployments.NewDeployment(validCreateDeployment(), now)
	if err != nil {
		t.Fatalf("NewDeployment() error = %v, want nil", err)
	}
	event, err := deployments.NewDeploymentCreatedEvent(deployment, now)
	if err != nil {
		t.Fatalf("NewDeploymentCreatedEvent() error = %v, want nil", err)
	}
	return deployment, event
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
