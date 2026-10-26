-- Index measurements for README question 3.
-- Runs inside a transaction that is rolled back at the end: nothing is left behind.
-- Synthetic data in tables with the same structure as the real ones.
\pset pager off
\set ON_ERROR_STOP on

BEGIN;

\echo
\echo === Generating 1,000,000 alarm rows and 1,000,000 point rows over 500 charts ===

CREATE TABLE zz_alarms (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chart_id      BIGINT           NOT NULL,
    point_seq     BIGINT           NOT NULL,
    rule          TEXT             NOT NULL,
    point_value   DOUBLE PRECISION NOT NULL,
    limit_version INTEGER          NOT NULL,
    UNIQUE (chart_id, point_seq, rule)
);

INSERT INTO zz_alarms (chart_id, point_seq, rule, point_value, limit_version)
SELECT (g % 500) + 1, (g / 500) + 1, 'trend_6', random() * 10, 1
FROM generate_series(0, 999999) AS g;

CREATE TABLE zz_points (
    chart_id  BIGINT             NOT NULL,
    seq       BIGINT             NOT NULL,
    subgroup  DOUBLE PRECISION[] NOT NULL,
    statistic DOUBLE PRECISION   NOT NULL
);

INSERT INTO zz_points (chart_id, seq, subgroup, statistic)
SELECT (g % 500) + 1, (g / 500) + 1, ARRAY[random() * 10], random() * 10
FROM generate_series(0, 999999) AS g;

ANALYZE zz_alarms;
ANALYZE zz_points;

\echo
\echo ==========================================================================
\echo INDEX 1: spc_alarms_chart_recent_idx on (chart_id, id DESC)
\echo Query: latest 50 alarms of one chart (chart detail screen)
\echo Each EXPLAIN runs twice; read the SECOND one (warm cache).
\echo ==========================================================================
\echo
\echo --- WITHOUT the index (only the primary key and the UNIQUE constraint exist) ---
EXPLAIN (ANALYZE, BUFFERS)
SELECT point_seq, rule, point_value, limit_version
FROM zz_alarms WHERE chart_id = 250 ORDER BY id DESC LIMIT 50;

EXPLAIN (ANALYZE, BUFFERS)
SELECT point_seq, rule, point_value, limit_version
FROM zz_alarms WHERE chart_id = 250 ORDER BY id DESC LIMIT 50;

CREATE INDEX zz_alarms_recent_idx ON zz_alarms (chart_id, id DESC);
ANALYZE zz_alarms;

\echo
\echo --- WITH the index ---
EXPLAIN (ANALYZE, BUFFERS)
SELECT point_seq, rule, point_value, limit_version
FROM zz_alarms WHERE chart_id = 250 ORDER BY id DESC LIMIT 50;

EXPLAIN (ANALYZE, BUFFERS)
SELECT point_seq, rule, point_value, limit_version
FROM zz_alarms WHERE chart_id = 250 ORDER BY id DESC LIMIT 50;

\echo
\echo ==========================================================================
\echo INDEX 2: spc_points primary key on (chart_id, seq)
\echo Query: last 8 points of one chart (read on every measurement)
\echo ==========================================================================
\echo
\echo --- WITHOUT the index ---
EXPLAIN (ANALYZE, BUFFERS)
SELECT seq, subgroup, statistic
FROM zz_points WHERE chart_id = 250 ORDER BY seq DESC LIMIT 8;

EXPLAIN (ANALYZE, BUFFERS)
SELECT seq, subgroup, statistic
FROM zz_points WHERE chart_id = 250 ORDER BY seq DESC LIMIT 8;

ALTER TABLE zz_points ADD PRIMARY KEY (chart_id, seq);
ANALYZE zz_points;

\echo
\echo --- WITH the primary key index ---
EXPLAIN (ANALYZE, BUFFERS)
SELECT seq, subgroup, statistic
FROM zz_points WHERE chart_id = 250 ORDER BY seq DESC LIMIT 8;

EXPLAIN (ANALYZE, BUFFERS)
SELECT seq, subgroup, statistic
FROM zz_points WHERE chart_id = 250 ORDER BY seq DESC LIMIT 8;

ROLLBACK;

\echo
\echo === Rolled back. Leftover measurement tables (must be 0): ===
SELECT count(*) AS leftover_tables FROM pg_tables WHERE tablename IN ('zz_alarms', 'zz_points');