CREATE TABLE IF NOT EXISTS locations (
    id          TEXT PRIMARY KEY,
    latitude    DOUBLE PRECISION NOT NULL,
    longitude   DOUBLE PRECISION NOT NULL,
    accuracy    REAL NOT NULL DEFAULT 0,
    live_period INTEGER NOT NULL DEFAULT 0,
    date        TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS locations_date_idx ON locations (date);
CREATE INDEX IF NOT EXISTS locations_recorded_at_idx ON locations (recorded_at DESC);
