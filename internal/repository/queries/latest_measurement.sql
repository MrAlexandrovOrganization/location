SELECT max(measurement_elapsed_ns) FROM public.locations WHERE measurement_key=$1 AND measurement_key<>'';
