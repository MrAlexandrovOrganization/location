-- Only new rejections made by the backend are evidence for automatic rules.
-- Do not reinterpret old hidden rows: they also include manual decisions.
CREATE TABLE public.location_outlier_observations (
    location_id TEXT PRIMARY KEY REFERENCES public.locations(id) ON DELETE CASCADE,
    reference_id TEXT REFERENCES public.locations(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX locations_coordinates_idx ON public.locations(latitude, longitude);
CREATE INDEX location_blacklist_exact_idx
    ON public.location_blacklist(latitude, longitude) WHERE enabled AND radius_m = 0;
CREATE INDEX location_blacklist_radius_idx
    ON public.location_blacklist(radius_m) WHERE enabled AND radius_m > 0;

CREATE FUNCTION public.location_distance_m(lat1 DOUBLE PRECISION, lon1 DOUBLE PRECISION,
                                           lat2 DOUBLE PRECISION, lon2 DOUBLE PRECISION)
RETURNS DOUBLE PRECISION LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $function$
    SELECT 12742000 * asin(sqrt(least(1.0, greatest(0.0,
        power(sin(radians(lat2 - lat1) / 2), 2)
        + cos(radians(lat1)) * cos(radians(lat2))
          * power(sin(radians(lon2 - lon1) / 2), 2)
    ))));
$function$;

-- Separate exact and radius lookups so the common exact path uses its index.
-- Backfilling a new rule must not scan the entire rule table for every point.
CREATE OR REPLACE FUNCTION public.location_blacklist_matches(point_lat DOUBLE PRECISION, point_lon DOUBLE PRECISION)
RETURNS BOOLEAN LANGUAGE sql STABLE STRICT
AS $function$
    SELECT EXISTS (
        SELECT FROM public.location_blacklist
        WHERE enabled AND radius_m = 0 AND latitude = point_lat AND longitude = point_lon
    ) OR EXISTS (
        SELECT FROM public.location_blacklist
        WHERE enabled AND radius_m > 0
          AND public.location_distance_m(latitude, longitude, point_lat, point_lon) <= radius_m
    );
$function$;
