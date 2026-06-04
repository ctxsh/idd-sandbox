package deployments

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreatePersistsDeploymentAndInitialEvent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	deployment, err := store.Create(ctx, validCreateDeployment())
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	found, err := store.FindByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil", err)
	}
	if found.ID != deployment.ID {
		t.Fatalf("found ID = %q, want %q", found.ID, deployment.ID)
	}
	if len(found.References) != 1 || found.References[0] != "https://github.com/org/repo/issues/123" {
		t.Fatalf("References = %#v, want original reference", found.References)
	}

	events, err := store.EventsByDeploymentID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("EventsByDeploymentID() error = %v, want nil", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != EventDeploymentCreated {
		t.Fatalf("event Type = %q, want %q", event.Type, EventDeploymentCreated)
	}
	if event.Actor != deployment.RequestedBy {
		t.Fatalf("event Actor = %q, want %q", event.Actor, deployment.RequestedBy)
	}
	if event.DeploymentID != deployment.ID {
		t.Fatalf("event DeploymentID = %q, want %q", event.DeploymentID, deployment.ID)
	}
}

func TestStoreReopenFindsDeployment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "deployments.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v, want nil", err)
	}
	deployment, err := store.Create(ctx, validCreateDeployment())
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() after reopen error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	})

	found, err := reopened.FindByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil", err)
	}
	if found.ID != deployment.ID {
		t.Fatalf("found ID = %q, want %q", found.ID, deployment.ID)
	}
}

func TestStoreEventsByDeploymentIDOldestFirst(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	deployment, err := store.Create(ctx, validCreateDeployment())
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	later := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	_, err = store.db.ExecContext(ctx, `
INSERT INTO deployment_events (id, deployment_id, type, actor, note, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, "evt_later", deployment.ID, "deployment_started", "alice", nil, later)
	if err != nil {
		t.Fatalf("insert later event: %v", err)
	}

	events, err := store.EventsByDeploymentID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("EventsByDeploymentID() error = %v, want nil", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].Type != EventDeploymentCreated || events[1].Type != "deployment_started" {
		t.Fatalf("event order = [%q, %q], want created then started", events[0].Type, events[1].Type)
	}
}

func TestStoreMissingIDsReturnNotFound(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.FindByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByID() error = %v, want ErrNotFound", err)
	}
	if _, err := store.EventsByDeploymentID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EventsByDeploymentID() error = %v, want ErrNotFound", err)
	}
}

func TestStoreCreateRollsBackWhenEventInsertFails(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `DROP TABLE deployment_events`)
	if err != nil {
		t.Fatalf("drop deployment_events: %v", err)
	}

	deployment, err := store.Create(ctx, validCreateDeployment())
	if err == nil {
		t.Fatal("Create() error = nil, want event insert failure")
	}
	if deployment.ID != "" {
		t.Fatalf("deployment ID = %q, want empty deployment on failure", deployment.ID)
	}

	var count int
	err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments`).Scan(&count)
	if err != nil {
		t.Fatalf("count deployments: %v", err)
	}
	if count != 0 {
		t.Fatalf("deployment rows after failed Create = %d, want 0", count)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "deployments.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	})
	return store
}
