-- Reapplying every rule to every visible location on each automatic exact rule
-- can exceed the mobile request deadline. Only the changed rules need backfill.
-- Existing rule/point locks and no-automatic-unhide semantics remain unchanged.
CREATE OR REPLACE FUNCTION public.location_blacklist_apply_rules()
RETURNS trigger LANGUAGE plpgsql VOLATILE AS $function$
BEGIN
    IF NOT EXISTS (SELECT FROM changed_rules WHERE enabled) THEN
        RETURN NULL;
    END IF;

    UPDATE public.locations l SET hidden = true
    FROM changed_rules b
    WHERE b.enabled AND b.radius_m = 0 AND NOT l.hidden
      AND l.latitude = b.latitude AND l.longitude = b.longitude;

    UPDATE public.locations l SET hidden = true
    FROM changed_rules b
    WHERE b.enabled AND b.radius_m > 0 AND NOT l.hidden
      -- Conservative latitude bounds, including circles at the poles/dateline.
      AND l.latitude BETWEEN b.latitude - b.radius_m / 100000
                         AND b.latitude + b.radius_m / 100000
      AND public.location_distance_m(b.latitude, b.longitude, l.latitude, l.longitude) <= b.radius_m;
    RETURN NULL;
END;
$function$;

DROP TRIGGER location_blacklist_rules_apply ON public.location_blacklist;

CREATE TRIGGER location_blacklist_rules_apply_insert
AFTER INSERT ON public.location_blacklist
REFERENCING NEW TABLE AS changed_rules
FOR EACH STATEMENT EXECUTE FUNCTION public.location_blacklist_apply_rules();

CREATE TRIGGER location_blacklist_rules_apply_update
AFTER UPDATE ON public.location_blacklist
REFERENCING NEW TABLE AS changed_rules
FOR EACH STATEMENT EXECUTE FUNCTION public.location_blacklist_apply_rules();
