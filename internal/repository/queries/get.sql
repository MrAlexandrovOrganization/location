SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at, provider, quality_reason,
       raw_payload, raw_sha256, measurement_key, measurement_elapsed_ns
FROM locations WHERE id=$1;
