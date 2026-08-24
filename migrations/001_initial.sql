CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('operator', 'partner')),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE INDEX sessions_user_active_idx ON sessions(user_id, expires_at, revoked_at);

CREATE TABLE compute_sites (
    id TEXT PRIMARY KEY,
    operator_id TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    country_code TEXT NOT NULL,
    timezone TEXT NOT NULL,
    total_units INTEGER NOT NULL CHECK (total_units > 0),
    available_units INTEGER NOT NULL CHECK (available_units >= 0 AND available_units <= total_units),
    status TEXT NOT NULL CHECK (status IN ('draft', 'verified', 'active', 'suspended')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(operator_id, name)
);
CREATE INDEX compute_sites_country_status_idx ON compute_sites(country_code, status);

CREATE TABLE compliance_reviews (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES compute_sites(id) ON DELETE CASCADE,
    reviewer_id TEXT NOT NULL REFERENCES users(id),
    decision TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    evidence_ref TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(site_id, id)
);

CREATE TABLE scenarios (
    id TEXT PRIMARY KEY,
    partner_id TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    sector TEXT NOT NULL,
    requested_units INTEGER NOT NULL CHECK (requested_units > 0),
    data_classification TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('submitted', 'reviewing', 'approved', 'rejected')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(partner_id, name)
);
CREATE INDEX scenarios_partner_status_idx ON scenarios(partner_id, status, created_at);

CREATE TABLE scenario_reviews (
    id TEXT PRIMARY KEY,
    scenario_id TEXT NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    reviewer_id TEXT NOT NULL REFERENCES users(id),
    decision TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    notes TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE reservations (
    id TEXT PRIMARY KEY,
    scenario_id TEXT NOT NULL REFERENCES scenarios(id),
    site_id TEXT NOT NULL REFERENCES compute_sites(id),
    partner_id TEXT NOT NULL REFERENCES users(id),
    units INTEGER NOT NULL CHECK (units > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'released', 'expired')),
    expires_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX reservations_site_status_idx ON reservations(site_id, status, expires_at);
CREATE INDEX reservations_partner_idx ON reservations(partner_id, created_at);

CREATE TABLE deployments (
    id TEXT PRIMARY KEY,
    reservation_id TEXT NOT NULL UNIQUE REFERENCES reservations(id),
    status TEXT NOT NULL CHECK (status IN ('queued', 'activating', 'running', 'stopping', 'stopped', 'failed')),
    endpoint TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    started_at TEXT,
    stopped_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE usage_records (
    id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL REFERENCES deployments(id),
    partner_id TEXT NOT NULL REFERENCES users(id),
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    unit_seconds INTEGER NOT NULL CHECK (unit_seconds >= 0),
    created_at TEXT NOT NULL,
    UNIQUE(deployment_id, period_start, period_end)
);

CREATE TABLE settlement_entries (
    id TEXT PRIMARY KEY,
    usage_id TEXT NOT NULL UNIQUE REFERENCES usage_records(id),
    partner_id TEXT NOT NULL REFERENCES users(id),
    amount_micros INTEGER NOT NULL CHECK (amount_micros >= 0),
    currency TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    available_at TEXT NOT NULL,
    lease_owner TEXT,
    lease_until TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(kind, aggregate_id)
);
CREATE INDEX jobs_claim_idx ON jobs(status, available_at, lease_until);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_id TEXT,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    result TEXT NOT NULL,
    request_id TEXT NOT NULL,
    details TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX audit_object_idx ON audit_events(object_type, object_id, created_at);

CREATE TABLE idempotency_records (
    scope TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idempotency_expiry_idx ON idempotency_records(expires_at);
