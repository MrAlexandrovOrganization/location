CREATE TABLE public.location_blacklist (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    latitude   DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude  DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_m   DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (radius_m BETWEEN 0 AND 100000),
    enabled    BOOLEAN NOT NULL DEFAULT true,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE public.location_blacklist IS
    'Permanent coordinate exclusions. Matching locations are retained with hidden=true.';
COMMENT ON COLUMN public.location_blacklist.radius_m IS
    'Radius in metres (0 = exact latitude/longitude match; maximum 100 km).';
COMMENT ON COLUMN public.location_blacklist.enabled IS
    'Disabled rules stop matching; previously hidden locations stay hidden.';

-- A spherical distance is sufficient for these user-defined exclusion circles.
-- Clamp the haversine argument to handle rounding at antipodal points.
CREATE FUNCTION public.location_blacklist_matches(point_lat DOUBLE PRECISION, point_lon DOUBLE PRECISION)
RETURNS BOOLEAN
LANGUAGE sql STABLE STRICT
AS $function$
    SELECT EXISTS (
        SELECT FROM public.location_blacklist b
        WHERE b.enabled AND (
            (b.radius_m = 0 AND b.latitude = point_lat AND b.longitude = point_lon)
            OR
            (b.radius_m > 0 AND 12742000 * asin(sqrt(least(1.0, greatest(0.0,
                power(sin(radians(point_lat - b.latitude) / 2), 2)
                + cos(radians(b.latitude)) * cos(radians(point_lat))
                  * power(sin(radians(point_lon - b.longitude) / 2), 2)
            )))) <= b.radius_m)
        )
    );
$function$;

-- Acquire locks before row locks. Multiple point writers may proceed together;
-- edits to rules wait for them, then backfill all committed matching points.
CREATE FUNCTION public.location_blacklist_lock_points()
RETURNS trigger LANGUAGE plpgsql VOLATILE AS $function$
BEGIN
    PERFORM pg_advisory_xact_lock_shared(736402812);
    RETURN NULL;
END;
$function$;

CREATE FUNCTION public.location_blacklist_lock_rules()
RETURNS trigger LANGUAGE plpgsql VOLATILE AS $function$
BEGIN
    PERFORM pg_advisory_xact_lock(736402812);
    RETURN NULL;
END;
$function$;

CREATE FUNCTION public.location_blacklist_hide_point()
RETURNS trigger LANGUAGE plpgsql VOLATILE AS $function$
BEGIN
    IF NOT NEW.hidden AND public.location_blacklist_matches(NEW.latitude, NEW.longitude) THEN
        NEW.hidden := true;
    END IF;
    RETURN NEW;
END;
$function$;

CREATE FUNCTION public.location_blacklist_apply_rules()
RETURNS trigger LANGUAGE plpgsql VOLATILE AS $function$
BEGIN
    UPDATE public.locations SET hidden = true
    WHERE NOT hidden AND public.location_blacklist_matches(latitude, longitude);
    RETURN NULL;
END;
$function$;

CREATE TRIGGER location_blacklist_points_lock
BEFORE INSERT OR UPDATE OF latitude, longitude, hidden ON public.locations
FOR EACH STATEMENT EXECUTE FUNCTION public.location_blacklist_lock_points();

CREATE TRIGGER location_blacklist_point
BEFORE INSERT OR UPDATE OF latitude, longitude, hidden ON public.locations
FOR EACH ROW EXECUTE FUNCTION public.location_blacklist_hide_point();

CREATE TRIGGER location_blacklist_rules_lock
BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON public.location_blacklist
FOR EACH STATEMENT EXECUTE FUNCTION public.location_blacklist_lock_rules();

-- Editing the table in DBeaver affects history in the same transaction. There
-- is deliberately no automatic unhide: manual and velocity exclusions survive.
CREATE TRIGGER location_blacklist_rules_apply
AFTER INSERT OR UPDATE ON public.location_blacklist
FOR EACH STATEMENT EXECUTE FUNCTION public.location_blacklist_apply_rules();
