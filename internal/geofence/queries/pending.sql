SELECT l.id,l.latitude,l.longitude,l.accuracy,l.recorded_at
FROM geo_pending q JOIN locations l ON l.id=q.location_id
WHERE NOT l.hidden AND l.quality_reason='' AND l.accuracy>0 AND l.accuracy<=100
AND l.recorded_at BETWEEN $1::timestamptz - interval '3 minutes' AND $1
ORDER BY l.recorded_at,l.id LIMIT 100
