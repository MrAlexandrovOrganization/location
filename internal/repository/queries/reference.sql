-- Do not reset continuity at UTC/date bucket boundaries. Network fixes keep
-- their own predecessor; GPS prefers a fresh, reasonably precise network fix.
WITH chosen AS MATERIALIZED (
    SELECT * FROM public.locations
    WHERE NOT hidden AND recorded_at < $1 AND accuracy BETWEEN 0 AND 500
    ORDER BY CASE
        WHEN $2 = 'network' AND provider = 'network' THEN 0
        WHEN $2 = 'gps' AND provider = 'network' AND accuracy > 0 AND accuracy <= 250
             AND recorded_at >= $1::timestamptz - interval '2 minutes' THEN 0
        ELSE 1 END,
        recorded_at DESC, id DESC
    LIMIT 1
)
SELECT c.id, c.latitude, c.longitude, c.accuracy, c.live_period, c.date, c.hidden,
       c.source::text, c.recorded_at, c.provider, c.quality_reason,
       c.accuracy > 0 AND c.accuracy <= 250
       AND c.recorded_at >= $1::timestamptz - interval '2 minutes'
       AND support.samples >= 2
       AND support.first_at <= c.recorded_at - interval '30 seconds' AS corroborated,
       c.accuracy > 0 AND c.accuracy <= 250
       AND c.recorded_at >= $1::timestamptz - interval '2 minutes'
       AND support.samples >= 1
       AND support.first_at <= c.recorded_at - interval '30 seconds' AS corroborated_pair
FROM chosen c
CROSS JOIN LATERAL (
    SELECT count(*) AS samples, min(p.recorded_at) AS first_at
    FROM public.locations p
    WHERE NOT p.hidden AND p.provider = c.provider
      AND p.accuracy > 0 AND p.accuracy <= 250
      AND p.recorded_at < c.recorded_at
      AND p.recorded_at >= c.recorded_at - interval '3 minutes'
      AND public.location_distance_m(p.latitude,p.longitude,c.latitude,c.longitude) <= 250
) support;
