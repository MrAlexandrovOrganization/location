-- Preserve all measurements and existing reasons. Only the diagnostic vocabulary
-- grows; earlier binaries remain compatible with the new schema.
ALTER TABLE public.locations
    DROP CONSTRAINT locations_quality_reason_check,
    ADD CONSTRAINT locations_quality_reason_check CHECK (
        quality_reason IN ('', 'speed', 'corroborated_jump', 'extreme_jump', 'uncertain_jump', 'poor_accuracy')
    );
