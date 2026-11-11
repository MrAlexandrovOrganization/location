SELECT p.id::text,p.latitude,p.longitude,p.radius_m,p.confirmation_seconds,p.version,
       s.state,s.candidate,s.candidate_since,s.last_observed_at,s.last_location_id
FROM geo_places p JOIN geo_states s ON s.place_id=p.id
WHERE p.enabled AND p.created_at <= $1 FOR UPDATE OF s
