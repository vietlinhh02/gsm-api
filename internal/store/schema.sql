CREATE TABLE IF NOT EXISTS services (
    id BIGINT PRIMARY KEY,
    display_name TEXT NOT NULL,
    service_type TEXT NOT NULL,
    travel_mode TEXT NOT NULL,
    capacity INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS access_tokens (
    token_hash TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    expires_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS access_tokens_expiry_idx ON access_tokens (expires_at);

CREATE TABLE IF NOT EXISTS estimates (
    fee_id TEXT PRIMARY KEY,
    environment TEXT NOT NULL DEFAULT 'production',
    client_id TEXT NOT NULL,
    service_id BIGINT NOT NULL REFERENCES services(id),
    service_type TEXT NOT NULL,
    order_time BIGINT NOT NULL,
    payment_method TEXT NOT NULL,
    addons JSONB NOT NULL DEFAULT '[]'::jsonb,
    points JSONB NOT NULL,
    amount BIGINT NOT NULL,
    route_distance DOUBLE PRECISION NOT NULL DEFAULT 0,
    route_duration DOUBLE PRECISION NOT NULL DEFAULT 0,
    response JSONB NOT NULL,
    expires_at BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS orders (
    id TEXT PRIMARY KEY,
    environment TEXT NOT NULL DEFAULT 'production',
    client_id TEXT NOT NULL,
    status TEXT NOT NULL,
    order_time BIGINT NOT NULL,
    payment_method TEXT NOT NULL,
    travel_mode TEXT NOT NULL,
    service_type TEXT NOT NULL,
    service_id BIGINT NOT NULL REFERENCES services(id),
    partner_trip_id TEXT NOT NULL DEFAULT '',
    driver_lock_time BIGINT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS orders_client_created_idx ON orders (client_id, created_at DESC);

CREATE TABLE IF NOT EXISTS webhook_events (
    event_id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS geocode_cache (
    cache_key TEXT PRIMARY KEY,
    response JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS sandbox_runs (
    order_id TEXT PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    resource_href TEXT NOT NULL,
    next_step_at TIMESTAMPTZ NOT NULL
);
