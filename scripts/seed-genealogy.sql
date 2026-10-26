-- Genealogy demo data: 1,000,000 lots, 2,100,000 edges, 600,000 shipped serials.
-- Deterministic (no random). Replaces existing genealogy data.
\set ON_ERROR_STOP on
BEGIN;

TRUNCATE shipment_items, shipments, lot_inputs, lots RESTART IDENTITY;

-- 100,000 raw lots (id 1..100000)
INSERT INTO lots (id, code, kind)
SELECT i + 1, 'RAW-' || lpad(i::text, 6, '0'), 'raw' FROM generate_series(0, 99999) AS i;

-- 300,000 intermediate lots (id 100001..400000)
INSERT INTO lots (id, code, kind)
SELECT 100001 + i, 'INT-' || lpad(i::text, 6, '0'), 'intermediate' FROM generate_series(0, 299999) AS i;

-- 600,000 finished serial numbers (id 400001..1000000)
INSERT INTO lots (id, code, kind)
SELECT 400001 + j, 'SN-' || lpad(j::text, 7, '0'), 'serial' FROM generate_series(0, 599999) AS j;

-- Each intermediate lot uses 3 raw lots.
INSERT INTO lot_inputs (output_lot_id, input_lot_id, quantity)
SELECT 100001 + i, 1 + ((i * 3 + k) % 100000), 1
FROM generate_series(0, 299999) AS i, generate_series(0, 2) AS k;

-- Each serial uses 2 intermediate lots.
INSERT INTO lot_inputs (output_lot_id, input_lot_id, quantity)
SELECT 400001 + j, 100001 + ((j * 2 + k) % 300000), 1
FROM generate_series(0, 599999) AS j, generate_series(0, 1) AS k;

-- 6,000 shipments to 40 customers, 100 serials each.
INSERT INTO shipments (id, code, customer, shipped_at)
SELECT s + 1, 'SHP-' || lpad(s::text, 5, '0'), 'MUSTERI-' || lpad((s % 40)::text, 2, '0'),
       timestamptz '2026-01-01 00:00:00+00' + s * interval '1 hour'
FROM generate_series(0, 5999) AS s;

INSERT INTO shipment_items (shipment_id, lot_id)
SELECT 1 + (j / 100), 400001 + j FROM generate_series(0, 599999) AS j;

SELECT setval(pg_get_serial_sequence('lots', 'id'), (SELECT max(id) FROM lots));
SELECT setval(pg_get_serial_sequence('shipments', 'id'), (SELECT max(id) FROM shipments));

COMMIT;
ANALYZE lots;
ANALYZE lot_inputs;
ANALYZE shipments;
ANALYZE shipment_items;

SELECT (SELECT count(*) FROM lots) AS lots,
       (SELECT count(*) FROM lot_inputs) AS edges,
       (SELECT count(*) FROM shipment_items) AS shipped_serials;