INSERT INTO location_measurement_context(location_id, device_id, status, version, data)
VALUES ($1, $2, $3, $4, $5::jsonb);
