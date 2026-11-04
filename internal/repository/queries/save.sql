INSERT INTO locations (id, latitude, longitude, accuracy, live_period, date, hidden, source, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::location_source, $9)
ON CONFLICT (id) DO NOTHING;
