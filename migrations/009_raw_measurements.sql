-- Raw input is immutable under UUID retries. Normalized columns and hidden/reason
-- are the derived view; rejected measurements remain available for reprocessing.
ALTER TABLE public.locations
    ADD COLUMN raw_payload JSONB,
    ADD COLUMN raw_sha256 TEXT NOT NULL DEFAULT '',
    ADD COLUMN measurement_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN measurement_elapsed_ns BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN received_at TIMESTAMPTZ,
    DROP CONSTRAINT locations_quality_reason_check,
    ADD CONSTRAINT locations_quality_reason_check CHECK (
        quality_reason IN ('', 'speed', 'corroborated_jump', 'extreme_jump', 'uncertain_jump', 'poor_accuracy',
                           'invalid_measurement', 'stale_measurement', 'duplicate_measurement', 'out_of_order_measurement')
    );
-- Older rows have unknown ingestion times, rather than a fabricated migration time.
ALTER TABLE public.locations ALTER COLUMN received_at SET DEFAULT now();
CREATE INDEX locations_measurement_order_idx
    ON public.locations(measurement_key, measurement_elapsed_ns DESC)
    WHERE measurement_key <> '';
