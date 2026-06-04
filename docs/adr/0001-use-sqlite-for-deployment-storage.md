# Use SQLite for deployment storage

The Deployment API stores deployments in a SQLite database. This keeps deployment records durable across process restarts while avoiding the operational weight of an external database for the example service.

## Consequences

- Deployment state survives process restarts.
- Local development and tests can run without provisioning external infrastructure.
- The storage layer should stay explicit enough to replace SQLite if the service later needs multi-instance writes or production database operations.
