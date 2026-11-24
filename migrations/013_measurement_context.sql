-- Optional context is stored atomically with the point, independently of quality policy.
CREATE TABLE location_measurement_context (
    location_id TEXT PRIMARY KEY REFERENCES locations(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('valid', 'invalid', 'unsupported_version', 'too_large')),
    version BIGINT,
    data JSONB,
    CHECK ((status = 'valid') = (data IS NOT NULL))
);
CREATE INDEX location_measurement_context_device_idx ON location_measurement_context(device_id);

-- Casts are restricted to the validated server projection; untrusted raw context stays in raw_payload.
CREATE VIEW location_context AS
SELECT l.id, l.recorded_at, l.latitude, l.longitude, l.accuracy, l.source, l.hidden, l.quality_reason,
       c.device_id, COALESCE(c.status, 'missing') AS context_status, c.version AS context_version,
       c.data #>> '{client,installation_id}' AS installation_id,
       c.data #>> '{client,app_version}' AS app_version,
       c.data #>> '{steps,status}' AS steps_status,
       (c.data #>> '{steps,count}')::bigint AS steps_count,
       (c.data #>> '{steps,from}')::timestamptz AS steps_from,
       (c.data #>> '{steps,to}')::timestamptz AS steps_to,
       extract(epoch FROM l.recorded_at - (c.data #>> '{steps,to}')::timestamptz) AS steps_age_s,
       c.data #>> '{movement,status}' AS movement_status,
       c.data #>> '{movement,event}' AS movement_event,
       (c.data #>> '{movement,last_event_at}')::timestamptz AS movement_at,
       extract(epoch FROM l.recorded_at - (c.data #>> '{movement,last_event_at}')::timestamptz) AS movement_age_s,
       c.data #>> '{collection,mode}' AS collection_mode,
       (c.data #>> '{collection,requested_interval_ms}')::bigint AS requested_interval_ms,
       c.data #>> '{platform,android,delivery_channel}' AS delivery_channel,
       c.data #>> '{platform,android,passive_reason}' AS passive_reason,
       c.data #>> '{platform,android,step_detector_status}' AS step_detector_status,
       c.data #>> '{platform,android,step_counter_status}' AS step_counter_status,
       l.raw_payload -> 'context' AS raw_context
FROM locations l LEFT JOIN location_measurement_context c ON c.location_id = l.id;
