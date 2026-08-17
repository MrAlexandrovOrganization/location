-- Location point source as a PostgreSQL enum: stored internally as a compact
-- 4-byte OID, surfaced as a readable label (e.g. 'telegram-bot').
-- New clients add values via: ALTER TYPE location_source ADD VALUE 'web';
DO $$ BEGIN
    CREATE TYPE location_source AS ENUM ('unknown', 'telegram-bot');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Idempotent: no-op when the column already exists (in any type).
ALTER TABLE locations ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'unknown';

-- Convert to the enum type; no-op when already an enum.
ALTER TABLE locations
    ALTER COLUMN source DROP DEFAULT,
    ALTER COLUMN source TYPE location_source USING source::location_source,
    ALTER COLUMN source SET DEFAULT 'unknown';
