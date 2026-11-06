INSERT INTO locations (id, latitude, longitude, accuracy, live_period, date, hidden, source, recorded_at, provider, quality_reason, raw_payload, raw_sha256, measurement_key, measurement_elapsed_ns)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::location_source, $9, COALESCE(NULLIF($10,''),'unknown'), $11, $12::jsonb, $13, $14, $15)
ON CONFLICT (id) DO NOTHING
RETURNING hidden;
