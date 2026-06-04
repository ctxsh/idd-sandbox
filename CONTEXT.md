# Deployment API

This context describes the lightweight deployment control plane. It records deployment intent, approval, lifecycle state, and review notes without running infrastructure changes.

## Language

**Deployment**: A long-lived record of one requested software deployment from initial request through approval, start, and terminal outcome.
_Avoid_: Deployment request as the object name after creation

**Risk**: A required production-only note describing the deployment risk being accepted.

**Rollback plan**: A required note describing how to reverse or recover from the deployment in any environment.

**Requester**: The person who asks for a deployment to be created.

**Reviewer**: A person other than the requester who approves or rejects a deployment.

**Review**: The single approval or rejection decision made for a deployment before it can start.

**Deployment event**: An immutable history entry recording a deployment lifecycle action, actor, timestamp, and optional note.

**Work reference**: An issue, pull request, or other planning reference connected to a deployment.

## Relationships

- A **Deployment** starts as a request and remains the same record as it moves through lifecycle states.
- A production **Deployment** must record **Risk** at creation time.
- Every **Deployment** must record a **Rollback plan** at creation time.
- A **Deployment** must include at least one **Work reference** at creation time.
- A **Requester** cannot approve their own **Deployment**.
- A **Review** records the reviewer and review time, with the deployment status carrying whether the decision was approval or rejection.
- A **Deployment** can be reviewed only while it is `pending_approval`.
- A **Deployment** can start only after it is `approved`.
- A **Deployment** can be rolled back after it has `started`, `succeeded`, or `failed`.
- `rejected` and `rolled_back` are terminal lifecycle states.
- `succeeded` and `failed` are final unless a rollback is recorded.
- Every creation, review, start, completion, failure, and rollback is recorded as a **Deployment event**.

## Example dialogue

> **Dev:** "Should starting an approved deployment create a separate run record?"
> **Domain expert:** "No — for this API, the **Deployment** itself is the record from request through outcome."
>
> **Dev:** "Do staging deployments need risk notes?"
> **Domain expert:** "No — only production deployments require **Risk**, but every deployment needs a **Rollback plan**."
>
> **Dev:** "Can the requester approve their own deployment?"
> **Domain expert:** "No — approval must come from a separate **Reviewer**."
>
> **Dev:** "Do we store separate approved-by and rejected-by fields?"
> **Domain expert:** "No — store the **Review** actor and timestamp once; the status says whether the deployment was approved or rejected."
>
> **Dev:** "Can a failed or succeeded deployment later be marked rolled back?"
> **Domain expert:** "Yes — rollback records that the team reversed a deployment after it started, regardless of whether it had succeeded or failed."
>
> **Dev:** "Where do we look to understand what happened over time?"
> **Domain expert:** "Read the **Deployment events**; the deployment record shows the current state, and the event history shows how it got there."

## Flagged ambiguities

- "deployment request" can sound like an approval-only object. Resolved: the canonical object is **Deployment**; requesting is the first lifecycle phase.
- Risk and rollback validation could happen at creation or start. Resolved: validate required notes at creation time only.
- `approved_by` does not leave room for rejection metadata. Resolved: use **Review** metadata (`reviewed_by`, `reviewed_at`) and let `status` represent approved versus rejected.
- Rollback transition semantics were ambiguous. Resolved: `rolled_back` can follow `started`, `succeeded`, or `failed`.
- `issue` was too narrow for linking planned work. Resolved: use **Work references** so deployments can point at issues, pull requests, or similar planning artifacts.
