-- Synthetic disposable fixture: rules and points occupy disjoint coordinates.
-- Skip per-row checks only while seeding; enable every trigger before the test.
ALTER TABLE public.locations DISABLE TRIGGER USER;
ALTER TABLE public.location_blacklist DISABLE TRIGGER USER;

INSERT INTO public.locations(id, latitude, longitude, accuracy, date, source, recorded_at)
SELECT 'synthetic-scale-' || n, 10 + n / 100000.0, 20 + n / 100000.0,
       5, '2026-01-01', 'android', '2026-01-01T00:00:00Z'::timestamptz + n * interval '1 second'
FROM generate_series(1, 32000) n;

INSERT INTO public.location_blacklist(latitude, longitude)
SELECT 40 + n / 100000.0, 100 + n / 100000.0
FROM generate_series(1, 1300) n;

ALTER TABLE public.locations ENABLE TRIGGER USER;
ALTER TABLE public.location_blacklist ENABLE TRIGGER USER;
ANALYZE public.locations;
ANALYZE public.location_blacklist;
