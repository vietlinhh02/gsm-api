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

CREATE TABLE IF NOT EXISTS simulated_drivers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    phone_number TEXT NOT NULL,
    travel_mode TEXT NOT NULL CHECK (travel_mode IN ('Car', 'Motorcycle')),
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    model TEXT NOT NULL,
    license_plate TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS driver_id TEXT REFERENCES simulated_drivers(id);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS quote_fee_id TEXT REFERENCES estimates(fee_id);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS booking_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS orders_quote_idx ON orders (quote_fee_id);
CREATE UNIQUE INDEX IF NOT EXISTS orders_active_driver_idx ON orders (environment, driver_id)
    WHERE status IN ('ASSIGNED', 'IN PROCESS');

CREATE TABLE IF NOT EXISTS webhook_events (
    event_id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS webhook_inboxes (
    id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    receive_token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_receipts (
    id BIGSERIAL PRIMARY KEY,
    inbox_id TEXT NOT NULL REFERENCES webhook_inboxes(id) ON DELETE CASCADE,
    headers JSONB NOT NULL,
    body TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS webhook_receipts_inbox_idx ON webhook_receipts (inbox_id, id DESC);
ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS target_url TEXT NOT NULL DEFAULT '';

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
