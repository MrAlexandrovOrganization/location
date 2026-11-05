INSERT INTO public.location_blacklist(latitude, longitude, radius_m, note)
SELECT latitude, longitude, 0, 'auto_speed_confirmed; source_id=' || $1
FROM public.locations WHERE id = $1;
