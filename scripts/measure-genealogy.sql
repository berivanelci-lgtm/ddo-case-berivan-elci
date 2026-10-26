\pset pager off
\set ON_ERROR_STOP on

\echo === Row counts ===
SELECT (SELECT count(*) FROM lots) AS lots,
       (SELECT count(*) FROM lot_inputs) AS edges,
       (SELECT count(*) FROM shipment_items) AS shipped_serials;

\echo
\echo === BACKWARD: serial SN-0300000 -> all input lots ===
EXPLAIN (ANALYZE, BUFFERS)
WITH RECURSIVE up(lot_id, depth) AS (
    SELECT input_lot_id, 1 FROM lot_inputs
    WHERE output_lot_id = (SELECT id FROM lots WHERE code = 'SN-0300000')
    UNION
    SELECT li.input_lot_id, up.depth + 1 FROM lot_inputs li JOIN up ON li.output_lot_id = up.lot_id WHERE up.depth < 20
)
SELECT l.id, l.code, l.kind, min(up.depth) FROM up JOIN lots l ON l.id = up.lot_id
GROUP BY l.id, l.code, l.kind ORDER BY 4, 2;

\echo
\echo === FORWARD: raw lot RAW-050000 -> all output lots ===
EXPLAIN (ANALYZE, BUFFERS)
WITH RECURSIVE down(lot_id, depth) AS (
    SELECT output_lot_id, 1 FROM lot_inputs
    WHERE input_lot_id = (SELECT id FROM lots WHERE code = 'RAW-050000')
    UNION
    SELECT li.output_lot_id, down.depth + 1 FROM lot_inputs li JOIN down ON li.input_lot_id = down.lot_id WHERE down.depth < 20
)
SELECT l.id, l.code, l.kind, min(down.depth) FROM down JOIN lots l ON l.id = down.lot_id
GROUP BY l.id, l.code, l.kind ORDER BY 4, 2;

\echo
\echo === RECALL: raw lot RAW-050000 -> affected shipments ===
EXPLAIN (ANALYZE, BUFFERS)
WITH RECURSIVE down(lot_id, depth) AS (
    SELECT (SELECT id FROM lots WHERE code = 'RAW-050000'), 0
    UNION
    SELECT li.output_lot_id, down.depth + 1 FROM lot_inputs li JOIN down ON li.input_lot_id = down.lot_id WHERE down.depth < 20
)
SELECT s.id, s.code, s.customer, s.shipped_at, count(DISTINCT si.lot_id)
FROM (SELECT DISTINCT lot_id FROM down) d
JOIN shipment_items si ON si.lot_id = d.lot_id
JOIN shipments s ON s.id = si.shipment_id
GROUP BY s.id, s.code, s.customer, s.shipped_at ORDER BY s.code;