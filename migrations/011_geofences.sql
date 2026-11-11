CREATE TABLE geo_places (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_m DOUBLE PRECISION NOT NULL CHECK (radius_m BETWEEN 100 AND 10000),
    confirmation_seconds INTEGER NOT NULL CHECK (confirmation_seconds BETWEEN 30 AND 600),
    version BIGINT NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE geo_states (
    place_id UUID PRIMARY KEY REFERENCES geo_places(id),
    state TEXT NOT NULL DEFAULT 'unknown' CHECK (state IN ('unknown','inside','outside')),
    candidate TEXT NOT NULL DEFAULT '',
    candidate_since TIMESTAMPTZ,
    last_observed_at TIMESTAMPTZ,
    last_location_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE geo_pending (
    location_id TEXT PRIMARY KEY
);
CREATE TABLE geo_events (
    sequence BIGSERIAL PRIMARY KEY,
    place_id UUID NOT NULL REFERENCES geo_places(id),
    place_version BIGINT NOT NULL,
    transition TEXT NOT NULL CHECK (transition IN ('entered','exited')),
    occurred_at TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ NOT NULL
);
-- An insertion and its durable evaluation intent always commit together.
-- No historical backfill: enabling the feature never replays old movements.
CREATE FUNCTION geo_enqueue() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.source = 'android' THEN
        INSERT INTO geo_pending(location_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER geo_enqueue AFTER INSERT ON locations FOR EACH ROW EXECUTE FUNCTION geo_enqueue();
