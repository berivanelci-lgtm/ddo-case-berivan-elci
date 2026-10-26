-- +goose Up
CREATE TABLE ncrs (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title             TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description       TEXT        NOT NULL DEFAULT '',
    severity          TEXT        NOT NULL CHECK (severity IN ('minor', 'major', 'critical')),
    status            TEXT        NOT NULL CHECK (status IN ('open', 'investigating', 'closed')),
    work_order_id     BIGINT      REFERENCES work_orders (id),
    chart_id          BIGINT      REFERENCES charts (id),
    opened_by         BIGINT      NOT NULL REFERENCES users (id),
    root_cause        TEXT,
    corrective_action TEXT,
    closed_by         BIGINT      REFERENCES users (id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at         TIMESTAMPTZ,
    -- Kapalı bir kayıt, kök neden ve düzeltici faaliyet olmadan var olamaz.
    CHECK (status <> 'closed' OR (root_cause IS NOT NULL AND corrective_action IS NOT NULL
                                  AND closed_by IS NOT NULL AND closed_at IS NOT NULL))
);

CREATE INDEX ncrs_status_idx ON ncrs (status, id DESC);

CREATE TRIGGER ncrs_audit AFTER INSERT OR UPDATE OR DELETE ON ncrs
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');

-- +goose Down
DROP TABLE ncrs;