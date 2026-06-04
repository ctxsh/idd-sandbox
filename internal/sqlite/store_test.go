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

func TestStoreListDeploymentsOrdersNewestFirst(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestDeployment(t, store, testDeployment("dep_old", "payments", "production", deployments.StatusPendingApproval, time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)))
	insertTestDeployment(t, store, testDeployment("dep_a", "payments", "production", deployments.StatusPendingApproval, time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)))
	insertTestDeployment(t, store, testDeployment("dep_z", "billing", "staging", deployments.StatusStarted, time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)))

	resp, err := store.ListDeployments(ctx, types.ListDeploymentsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListDeployments() error = %v, want nil", err)
	}
	assertDeploymentIDs(t, resp.Items, []string{"dep_z", "dep_a", "dep_old"})
	if resp.NextOffset != nil {
		t.Fatalf("NextOffset = %v, want nil", *resp.NextOffset)
	}
}

func TestStoreListDeploymentsFilters(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	insertTestDeployment(t, store, testDeployment("dep_payments_prod_pending", "payments", "production", deployments.StatusPendingApproval, base))
	insertTestDeployment(t, store, testDeployment("dep_payments_staging_started", "payments", "staging", deployments.StatusStarted, base.Add(time.Minute)))
	insertTestDeployment(t, store, testDeployment("dep_billing_prod_started", "billing", "production", deployments.StatusStarted, base.Add(2*time.Minute)))

	tests := []struct {
		name string
		req  types.ListDeploymentsRequest
		want []string
	}{
		{
			name: "service",
			req:  types.ListDeploymentsRequest{Service: "payments", Limit: 10},
			want: []string{"dep_payments_staging_started", "dep_payments_prod_pending"},
		},
		{
			name: "environment",
			req:  types.ListDeploymentsRequest{Environment: "production", Limit: 10},
			want: []string{"dep_billing_prod_started", "dep_payments_prod_pending"},
		},
		{
			name: "status",
			req:  types.ListDeploymentsRequest{Status: deployments.StatusStarted, Limit: 10},
			want: []string{"dep_billing_prod_started", "dep_payments_staging_started"},
		},
		{
			name: "combined",
			req: types.ListDeploymentsRequest{
				Service:     "payments",
				Environment: "staging",
				Status:      deployments.StatusStarted,
				Limit:       10,
			},
			want: []string{"dep_payments_staging_started"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := store.ListDeployments(ctx, tt.req)
			if err != nil {
				t.Fatalf("ListDeployments() error = %v, want nil", err)
			}
			assertDeploymentIDs(t, resp.Items, tt.want)
		})
	}
}

func TestStoreListDeploymentsPaginates(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	insertTestDeployment(t, store, testDeployment("dep_1", "payments", "production", deployments.StatusPendingApproval, base))
	insertTestDeployment(t, store, testDeployment("dep_2", "payments", "production", deployments.StatusPendingApproval, base.Add(time.Minute)))
	insertTestDeployment(t, store, testDeployment("dep_3", "payments", "production", deployments.StatusPendingApproval, base.Add(2*time.Minute)))

	first, err := store.ListDeployments(ctx, types.ListDeploymentsRequest{Limit: 2})
	if err != nil {
		t.Fatalf("ListDeployments() first page error = %v, want nil", err)
	}
	assertDeploymentIDs(t, first.Items, []string{"dep_3", "dep_2"})
	if first.NextOffset == nil || *first.NextOffset != 2 {
		t.Fatalf("first NextOffset = %v, want 2", first.NextOffset)
	}

	second, err := store.ListDeployments(ctx, types.ListDeploymentsRequest{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListDeployments() second page error = %v, want nil", err)
	}
	assertDeploymentIDs(t, second.Items, []string{"dep_1"})
	if second.NextOffset != nil {
		t.Fatalf("second NextOffset = %v, want nil", *second.NextOffset)
	}

	empty, err := store.ListDeployments(ctx, types.ListDeploymentsRequest{Limit: 2, Offset: 10})
	if err != nil {
		t.Fatalf("ListDeployments() empty page error = %v, want nil", err)
	}
	assertDeploymentIDs(t, empty.Items, []string{})
	if empty.NextOffset != nil {
		t.Fatalf("empty NextOffset = %v, want nil", *empty.NextOffset)
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

func testDeployment(id string, service string, environment string, status string, createdAt time.Time) types.Deployment {
	risk := "customer-facing rollout"
	return types.Deployment{
		ID:           id,
		Service:      service,
		Environment:  environment,
		Version:      "v1.2.3",
		Status:       status,
		RequestedBy:  "alice",
		References:   []string{"https://github.com/org/repo/issues/123"},
		Risk:         &risk,
		RollbackPlan: "revert to v1.2.2",
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
}

func insertTestDeployment(t *testing.T, store *Store, deployment types.Deployment) {
	t.Helper()
	event := types.DeploymentEvent{
		ID:           "evt_" + deployment.ID,
		DeploymentID: deployment.ID,
		Type:         deployments.EventDeploymentCreated,
		Actor:        deployment.RequestedBy,
		CreatedAt:    deployment.CreatedAt,
	}
	if err := store.CreateDeployment(context.Background(), deployment, event); err != nil {
		t.Fatalf("CreateDeployment(%s) error = %v, want nil", deployment.ID, err)
	}
}

func assertDeploymentIDs(t *testing.T, deployments []types.Deployment, want []string) {
	t.Helper()
	if len(deployments) != len(want) {
		t.Fatalf("deployment len = %d, want %d (%#v)", len(deployments), len(want), deployments)
	}
	for i, deployment := range deployments {
		if deployment.ID != want[i] {
			t.Fatalf("deployment[%d].ID = %q, want %q", i, deployment.ID, want[i])
		}
	}
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
