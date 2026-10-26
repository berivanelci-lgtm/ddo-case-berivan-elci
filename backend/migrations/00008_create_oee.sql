-- +goose Up
CREATE TABLE lines (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code           TEXT        NOT NULL UNIQUE CHECK (char_length(code) BETWEEN 1 AND 40),
    name           TEXT        NOT NULL,
    ideal_cycle_ms BIGINT      NOT NULL CHECK (ideal_cycle_ms > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE downtime_reasons (
    code        TEXT    PRIMARY KEY,
    description TEXT    NOT NULL,
    planned     BOOLEAN NOT NULL
);

INSERT INTO downtime_reasons (code, description, planned) VALUES
    ('PLANNED_MAINTENANCE', 'Planlı bakım', true),
    ('BREAK', 'Mola', true),
    ('BREAKDOWN', 'Arıza', false),
    ('MATERIAL_WAIT', 'Malzeme bekleme', false),
    ('SETUP', 'Ayar / model değişimi', false),
    ('QUALITY_HOLD', 'Kalite beklemesi', false);

CREATE TABLE downtime_events (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    line_id     BIGINT      NOT NULL REFERENCES lines (id),
    reason_code TEXT        NOT NULL REFERENCES downtime_reasons (code),
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL,
    recorded_by BIGINT      NOT NULL REFERENCES users (id),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_at > started_at),
    CHECK (ended_at - started_at <= interval '24 hours')
);

CREATE INDEX downtime_events_line_start_idx ON downtime_events (line_id, started_at);

CREATE TABLE production_counts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    line_id     BIGINT      NOT NULL REFERENCES lines (id),
    occurred_at TIMESTAMPTZ NOT NULL,
    good        BIGINT      NOT NULL CHECK (good >= 0),
    scrap       BIGINT      NOT NULL CHECK (scrap >= 0),
    recorded_by BIGINT      NOT NULL REFERENCES users (id),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (good + scrap > 0)
);

CREATE INDEX production_counts_line_time_idx ON production_counts (line_id, occurred_at);

CREATE TRIGGER lines_audit AFTER INSERT OR UPDATE OR DELETE ON lines
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');
CREATE TRIGGER downtime_events_audit AFTER INSERT OR UPDATE OR DELETE ON downtime_events
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');

-- +goose Down
DROP TABLE production_counts;
DROP TABLE downtime_events;
DROP TABLE downtime_reasons;
DROP TABLE lines;