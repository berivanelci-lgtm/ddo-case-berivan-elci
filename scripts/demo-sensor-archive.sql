-- T-5 proof: monthly partitions, archiving of old partitions, partition pruning.
-- Safe to run repeatedly: earlier archived demo partitions are removed first.
\pset pager off
\set ON_ERROR_STOP on

DROP TABLE IF EXISTS archive.sensor_readings_2026_07, archive.sensor_readings_2026_08;

SELECT ensure_sensor_partition('2026-07-15'), ensure_sensor_partition('2026-08-15');

INSERT INTO sensor_readings (station_id, recorded_at, value)
SELECT 1 + (g % 50), timestamptz '2026-07-01 00:00:00+00' + g * interval '1 minute', g
FROM generate_series(0, 999) AS g;
INSERT INTO sensor_readings (station_id, recorded_at, value)
SELECT 1 + (g % 50), timestamptz '2026-08-01 00:00:00+00' + g * interval '1 minute', g
FROM generate_series(0, 999) AS g;

\echo
\echo === Partitions BEFORE archiving ===
SELECT c.relname AS partition, pg_get_expr(c.relpartbound, c.oid) AS bounds
FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = 'public.sensor_readings'::regclass ORDER BY 1;
SELECT count(*) AS rows_before_september FROM sensor_readings WHERE recorded_at < '2026-09-01';

\echo
\echo === Archive partitions that ended on or before 2026-09-01 ===
SELECT * FROM archive_sensor_partitions('2026-09-01');

\echo
\echo === Partitions AFTER archiving ===
SELECT c.relname AS partition, pg_get_expr(c.relpartbound, c.oid) AS bounds
FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = 'public.sensor_readings'::regclass ORDER BY 1;
SELECT count(*) AS rows_before_september FROM sensor_readings WHERE recorded_at < '2026-09-01';
SELECT (SELECT count(*) FROM archive.sensor_readings_2026_07) AS archived_july,
       (SELECT count(*) FROM archive.sensor_readings_2026_08) AS archived_august;

\echo
\echo === Critical query: last hour of one station (partition pruning + index) ===
EXPLAIN (ANALYZE, BUFFERS)
SELECT recorded_at, value FROM sensor_readings
WHERE station_id = 7 AND recorded_at >= now() - interval '1 hour'
ORDER BY recorded_at DESC LIMIT 100;