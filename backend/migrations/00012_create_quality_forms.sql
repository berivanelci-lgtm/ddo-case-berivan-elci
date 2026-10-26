-- +goose Up
CREATE TABLE quality_forms (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code            TEXT        NOT NULL UNIQUE CHECK (char_length(code) BETWEEN 1 AND 40),
    name            TEXT        NOT NULL,
    gauge_code      TEXT        NOT NULL DEFAULT '',
    calibration_due DATE,
    created_by      BIGINT      NOT NULL REFERENCES users (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE quality_form_items (
    form_id        BIGINT           NOT NULL REFERENCES quality_forms (id),
    position       INTEGER          NOT NULL CHECK (position > 0),
    characteristic TEXT             NOT NULL,
    unit           TEXT             NOT NULL DEFAULT '',
    nominal        DOUBLE PRECISION NOT NULL,
    lsl            DOUBLE PRECISION NOT NULL,
    usl            DOUBLE PRECISION NOT NULL,
    chart_id       BIGINT           REFERENCES charts (id),
    PRIMARY KEY (form_id, position),
    CHECK (lsl < usl AND nominal BETWEEN lsl AND usl)
);

CREATE TABLE quality_inspections (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    form_id       BIGINT      NOT NULL REFERENCES quality_forms (id),
    inspected_by  BIGINT      NOT NULL REFERENCES users (id),
    inspected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    work_order_id BIGINT      REFERENCES work_orders (id),
    all_in_spec   BOOLEAN     NOT NULL
);

CREATE INDEX quality_inspections_form_idx ON quality_inspections (form_id, id DESC);

CREATE TABLE quality_inspection_values (
    inspection_id BIGINT           NOT NULL REFERENCES quality_inspections (id),
    position      INTEGER          NOT NULL,
    value         DOUBLE PRECISION NOT NULL,
    in_spec       BOOLEAN          NOT NULL,
    PRIMARY KEY (inspection_id, position)
);

CREATE TRIGGER quality_forms_audit AFTER INSERT OR UPDATE OR DELETE ON quality_forms
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');

-- +goose Down
DROP TABLE quality_inspection_values;
DROP TABLE quality_inspections;
DROP TABLE quality_form_items;
DROP TABLE quality_forms;