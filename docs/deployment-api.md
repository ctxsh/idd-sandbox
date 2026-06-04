# Deployment API

## Summary

Build a small HTTP API that lets a team request, approve, track, and audit
software deployments. The API is a lightweight control plane: it records
deployment intent, review decisions, lifecycle state, and history, but it does
not actually deploy infrastructure.

## Problem

Teams often coordinate deployments across issues, pull requests, chat, and
manual checklists. That makes it hard to answer basic questions:

- What is being deployed?
- Who requested it?
- Has it been approved?
- What risk was accepted?
- What is the rollback plan?
- What happened after the deployment started?
- Who changed deployment state, and when?

This project should make those answers explicit and reviewable.

## Target Users

- Engineers requesting a deployment.
- Reviewers approving or rejecting a deployment.
- On-call engineers checking current deployment state.

## Initial Scope

The API should support:

- Creating a deployment.
- Listing deployments.
- Fetching one deployment.
- Approving or rejecting a deployment.
- Moving an approved deployment through started, succeeded, failed, and rolled
  back states.
- Recording issue, pull request, or planning references on the deployment.
- Recording risk notes for production deployments.
- Recording rollback notes for every deployment.
- Recording lifecycle events for deployment history and auditability.

## Out Of Scope

- Running real deploys.
- Integrating with Kubernetes, cloud APIs, or CI systems.
- Full authentication and authorization.
- Web UI.
- Multi-service orchestration.

## Deployment Model

A deployment should include:

- `id`
- `service`
- `environment`
- `version`
- `status`
- `requested_by`
- `reviewed_by`
- `reviewed_at`
- `references`
- `risk`
- `rollback_plan`
- `created_at`
- `updated_at`
- `started_at`
- `completed_at`
- `rolled_back_at`

Server-generated fields:

- `id`
- timestamps
- lifecycle status
- review metadata

Client-provided creation fields:

- `service`
- `environment`
- `version`
- `requested_by`
- `references`
- `risk`
- `rollback_plan`

Creation validation:

- `service`, `environment`, `version`, and `requested_by` are required.
- `references` must include at least one issue, pull request, or planning
  reference.
- `rollback_plan` is required for all environments.
- `risk` is required only when `environment` is `production`.

Because full authentication and authorization are out of scope, action endpoints
accept an explicit actor field in the request body.

Possible statuses:

- `pending_approval`
- `approved`
- `rejected`
- `started`
- `succeeded`
- `failed`
- `rolled_back`

Valid lifecycle transitions:

- `pending_approval -> approved`
- `pending_approval -> rejected`
- `approved -> started`
- `started -> succeeded`
- `started -> failed`
- `started -> rolled_back`
- `succeeded -> rolled_back`
- `failed -> rolled_back`

`rejected` and `rolled_back` are terminal states. `succeeded` and `failed` are
final unless a rollback is recorded.

## Deployment Events

Every creation and lifecycle action should create a deployment event.

A deployment event should include:

- `id`
- `deployment_id`
- `type`
- `actor`
- `note`
- `created_at`

Possible event types:

- `deployment_created`
- `deployment_approved`
- `deployment_rejected`
- `deployment_started`
- `deployment_succeeded`
- `deployment_failed`
- `deployment_rolled_back`

Reject, fail, and rollback actions require a non-empty `note`. Approve, start,
and succeed actions may include a note.

Events are append-only and returned oldest first for a deployment.

## Candidate API

- `POST /deployments`
- `GET /deployments`
- `GET /deployments/{id}`
- `GET /deployments/{id}/events`
- `POST /deployments/{id}/approve`
- `POST /deployments/{id}/reject`
- `POST /deployments/{id}/start`
- `POST /deployments/{id}/succeed`
- `POST /deployments/{id}/fail`
- `POST /deployments/{id}/rollback`

`GET /deployments` should support filtering by `service`, `environment`, and
`status`, with limit-based pagination.

Invalid lifecycle transitions should return an explicit conflict error. Missing
or invalid request data should return a validation error.

## Storage Model

Use SQLite for durable storage.

The database should have:

- a `deployments` table for current deployment state
- a `deployment_events` table for append-only deployment history

Lifecycle writes should validate the current state, update the deployment row,
and append the corresponding event in one transaction.

## Design Constraints

- Prefer clear lifecycle rules over infrastructure realism.
- Make invalid state transitions explicit errors.
- Make work references visible in API responses so PRs can connect back to the
  planned work.
- Preserve lifecycle history without requiring event replay for ordinary list
  and fetch requests.
