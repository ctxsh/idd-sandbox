# Store current state and event history

The Deployment API stores each deployment's current state in a `deployments` table and stores lifecycle history in an append-only `deployment_events` table. Each lifecycle write validates the current state, updates the deployment row, and inserts the corresponding event in one SQLite transaction.

## Consequences

- List and fetch endpoints can read the current deployment state without replaying history.
- The event log preserves reviewable history for creation, approval, rejection, start, success, failure, and rollback actions.
- The service is not fully event-sourced; the current state row is maintained directly and can be checked against event history if needed.
