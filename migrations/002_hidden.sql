ALTER TABLE locations ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS locations_visible_idx ON locations (date, recorded_at)
    WHERE NOT hidden;
