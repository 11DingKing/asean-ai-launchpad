# ASEAN AI Launchpad

ASEAN AI Launchpad is a production-oriented Go backend for coordinating compute sites, AI scenario qualification, capacity reservation, deployment rollout, metering, settlement, and durable operational recovery across regional partners.

## Core workflows

1. Operators register and verify compute sites, then activate capacity for approved workloads.
2. Partners submit AI scenarios for review, reserve capacity after approval, and track deployment activation.
3. Meter reports settle usage into an append-only ledger while durable workers expire reservations and execute deployment jobs.

The service uses SQLite with versioned migrations, foreign keys, conditional updates, transactional outbox jobs, opaque revocable sessions, role-aware authorization, request IDs, structured logs, and health/readiness endpoints.

## Run locally

```sh
cp .env.example .env
go run ./cmd/server
```

The bootstrap operator is created only when both bootstrap environment variables are set. Use `POST /v1/auth/login` to obtain an opaque bearer token. Health is available at `/healthz`; readiness additionally verifies database access at `/readyz`.

Capacity reservation and deployment creation require an `Idempotency-Key` header of 8 to 200 characters. Replaying the same actor, method, path, key, and body returns the stored successful response with `Idempotency-Replayed: true`; reusing a key with a different body is rejected.

## Quality gates

```sh
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

Build the container with `docker build --platform linux/amd64 .` or `docker build --platform linux/arm64 .`. The default entrypoint is the real `cmd/server` binary and stores data under `/data`.
