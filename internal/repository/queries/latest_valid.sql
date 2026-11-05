SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at
FROM public.locations
WHERE date = $1 AND NOT hidden AND recorded_at < $2
ORDER BY recorded_at DESC, id DESC
LIMIT 1;
