ALTER TABLE public.locations
    ADD COLUMN provider TEXT NOT NULL DEFAULT 'unknown'
        CHECK (provider IN ('unknown', 'gps', 'network', 'passive', 'fused')),
    ADD COLUMN quality_reason TEXT NOT NULL DEFAULT ''
        CHECK (quality_reason IN ('', 'speed', 'corroborated_jump', 'uncertain_jump', 'poor_accuracy'));

CREATE INDEX locations_provider_time_idx
    ON public.locations(provider, recorded_at DESC) WHERE NOT hidden;
