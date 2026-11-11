SELECT p.id::text,p.name,p.latitude,p.longitude,p.radius_m,p.confirmation_seconds,p.version,
       CASE WHEN s.last_observed_at IS NULL OR s.last_observed_at < now()-interval '5 minutes' THEN 'unknown' ELSE s.state END,
       s.last_observed_at
FROM geo_places p JOIN geo_states s ON s.place_id=p.id
WHERE p.enabled ORDER BY p.name,p.id
