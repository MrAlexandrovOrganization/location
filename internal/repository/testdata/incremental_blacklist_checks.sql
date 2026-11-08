INSERT INTO locations(id,latitude,longitude,date) VALUES
    ('synthetic-dateline',10,-179.999,'2026-01-01'),
    ('synthetic-polar',89.999,180,'2026-01-01'),
    ('synthetic-radius-outside',10,-179.5,'2026-01-01');

-- Fail deterministically if a rule change invokes the global matcher for all
-- historical points. Updates setting hidden=true must not recheck old rules.
CREATE OR REPLACE FUNCTION public.location_blacklist_matches(point_lat double precision, point_lon double precision)
RETURNS boolean LANGUAGE plpgsql STABLE STRICT AS $function$
BEGIN
    RAISE EXCEPTION 'rule backfill unexpectedly consulted the entire blacklist';
END;
$function$;

INSERT INTO location_blacklist(latitude,longitude,radius_m,enabled) VALUES
    (10.00001,20.00001,0,true), (10.1,20.1,10,true), (10.2,20.2,0,false),
    (10,179.999,500,true), (89.999,0,300,true);

DO $check$
BEGIN
    IF NOT (SELECT hidden FROM locations WHERE id='synthetic-scale-1')
       OR NOT (SELECT hidden FROM locations WHERE id='synthetic-scale-10000')
       OR (SELECT hidden FROM locations WHERE id='synthetic-scale-20000')
       OR (SELECT hidden FROM locations WHERE id='synthetic-scale-30000')
       OR NOT (SELECT hidden FROM locations WHERE id='synthetic-dateline')
       OR NOT (SELECT hidden FROM locations WHERE id='synthetic-polar')
       OR (SELECT hidden FROM locations WHERE id='synthetic-radius-outside') THEN
        RAISE EXCEPTION 'wrong bulk backfill: exact/radius/disabled/polar/dateline/outside';
    END IF;
END;
$check$;

UPDATE location_blacklist SET enabled=true,
    latitude=CASE WHEN latitude=10.00001 THEN 10.3 ELSE latitude END,
    longitude=CASE WHEN longitude=20.00001 THEN 20.3 ELSE longitude END
WHERE latitude IN (10.00001,10.2);

DO $check$
BEGIN
    IF NOT (SELECT hidden FROM locations WHERE id='synthetic-scale-1')
       OR NOT (SELECT hidden FROM locations WHERE id='synthetic-scale-20000')
       OR NOT (SELECT hidden FROM locations WHERE id='synthetic-scale-30000') THEN
        RAISE EXCEPTION 'bulk rule update lost prior hiding or missed changed coordinates';
    END IF;
END;
$check$;
