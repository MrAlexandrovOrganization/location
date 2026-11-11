DELETE FROM geo_pending q WHERE NOT EXISTS (
 SELECT 1 FROM locations l WHERE l.id=q.location_id
 AND NOT l.hidden AND l.quality_reason='' AND l.accuracy>0 AND l.accuracy<=100
 AND l.recorded_at BETWEEN $1::timestamptz - interval '3 minutes' AND $1::timestamptz + interval '30 seconds'
)
