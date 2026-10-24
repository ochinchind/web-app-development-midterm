CREATE TABLE IF NOT EXISTS organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations (id) ON DELETE SET NULL,
    name            TEXT        NOT NULL,
    email           TEXT UNIQUE,
    phone           TEXT UNIQUE,
    password_hash   TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS vehicles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    plate_number TEXT        NOT NULL UNIQUE,
    model        TEXT        NOT NULL DEFAULT '',
    capacity_kg  INTEGER     NOT NULL CHECK (capacity_kg > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by        UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id   UUID REFERENCES organizations (id) ON DELETE SET NULL,
    from_address      TEXT        NOT NULL,
    to_address        TEXT        NOT NULL,
    cargo_description TEXT        NOT NULL DEFAULT '',
    weight_kg         INTEGER     NOT NULL CHECK (weight_kg > 0),
    status            TEXT        NOT NULL DEFAULT 'NEW' CHECK (status IN ('NEW', 'TAKEN', 'DELIVERED')),
    driver_id         UUID REFERENCES users (id) ON DELETE SET NULL,
    vehicle_id        UUID REFERENCES vehicles (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    taken_at          TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS orders_status_idx ON orders (status);
