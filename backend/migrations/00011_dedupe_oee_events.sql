-- +goose Up
-- Tekrar gönderilen olaylar OEE'de iki kez sayılmasın: aynı hat ve aynı andaki sayaç,
-- aynı hat, neden ve zaman aralığındaki duruş tek kayıt olarak tutulur.
DELETE FROM production_counts a USING production_counts b
WHERE a.line_id = b.line_id AND a.occurred_at = b.occurred_at AND a.id > b.id;

DELETE FROM downtime_events a USING downtime_events b
WHERE a.line_id = b.line_id AND a.reason_code = b.reason_code
  AND a.started_at = b.started_at AND a.ended_at = b.ended_at AND a.id > b.id;

ALTER TABLE production_counts
    ADD CONSTRAINT production_counts_line_time_key UNIQUE (line_id, occurred_at);
ALTER TABLE downtime_events
    ADD CONSTRAINT downtime_events_dedupe_key UNIQUE (line_id, reason_code, started_at, ended_at);

-- +goose Down
ALTER TABLE downtime_events DROP CONSTRAINT downtime_events_dedupe_key;
ALTER TABLE production_counts DROP CONSTRAINT production_counts_line_time_key;