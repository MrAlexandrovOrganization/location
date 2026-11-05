-- $1: newly stored observation, $2: minimum samples, $3: distinct Moscow days,
-- $4: neighbourhood radius in metres. Rules themselves always remain exact.
WITH candidate AS (
    SELECT latitude, longitude
    FROM public.locations WHERE id = $1 AND hidden
), evidence AS (
    SELECT count(*) AS samples,
           count(DISTINCT (l.recorded_at AT TIME ZONE 'Europe/Moscow')::date) AS days
    FROM public.location_outlier_observations o
    JOIN public.locations l ON l.id = o.location_id
    CROSS JOIN candidate c
    WHERE l.hidden
      AND l.latitude BETWEEN c.latitude - $4::double precision / 100000
                         AND c.latitude + $4::double precision / 100000
      AND public.location_distance_m(c.latitude, c.longitude, l.latitude, l.longitude) <= $4
)
SELECT EXISTS (
    SELECT FROM candidate c CROSS JOIN evidence e
    WHERE e.samples >= $2 AND e.days >= $3
      AND NOT EXISTS (
          SELECT FROM public.locations l
          WHERE NOT l.hidden
            AND l.latitude BETWEEN c.latitude - $4::double precision / 100000
                               AND c.latitude + $4::double precision / 100000
            AND public.location_distance_m(c.latitude, c.longitude, l.latitude, l.longitude) <= $4
      )
      -- A disabled matching rule is a manual opt-out, not an invitation to re-add it.
      AND NOT EXISTS (
          SELECT FROM public.location_blacklist b
          WHERE (b.radius_m = 0 AND b.latitude = c.latitude AND b.longitude = c.longitude)
             OR (b.radius_m > 0
                 AND public.location_distance_m(b.latitude, b.longitude, c.latitude, c.longitude) <= b.radius_m)
      )
);
