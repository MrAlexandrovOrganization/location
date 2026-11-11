SELECT sequence,place_id::text,place_version,transition,occurred_at,confirmed_at
FROM geo_events WHERE sequence>$1 ORDER BY sequence LIMIT $2
