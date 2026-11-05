-- Take the exclusive rule lock before the point trigger takes its shared lock.
-- This prevents lock upgrades between concurrent outlier writers.
SELECT pg_advisory_xact_lock(736402812);
