-- +goose Up
CREATE TABLE audit_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id     BIGINT REFERENCES users (id),
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   BIGINT      NOT NULL,
    old_value   JSONB,
    new_value   JSONB
);

CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, id DESC);

-- Kayıtları yazan genel tetikleyici işlevi. İzlenen tablonun kimlik sütununun adı
-- tetikleyiciye argüman olarak verilir. user_id boşsa işlem veritabanında doğrudan
-- (API dışında) yapılmıştır.
-- +goose StatementBegin
CREATE FUNCTION audit_row() RETURNS trigger AS $$
DECLARE
    old_row JSONB;
    new_row JSONB;
    row_id  BIGINT;
    actor   BIGINT;
BEGIN
    actor := NULLIF(current_setting('app.user_id', true), '')::BIGINT;

    IF TG_OP = 'INSERT' THEN
        new_row := to_jsonb(NEW);
        row_id := (new_row ->> TG_ARGV[0])::BIGINT;
    ELSIF TG_OP = 'UPDATE' THEN
        old_row := to_jsonb(OLD);
        new_row := to_jsonb(NEW);
        row_id := (new_row ->> TG_ARGV[0])::BIGINT;
    ELSE
        old_row := to_jsonb(OLD);
        row_id := (old_row ->> TG_ARGV[0])::BIGINT;
    END IF;

    INSERT INTO audit_log (user_id, action, entity_type, entity_id, old_value, new_value)
    VALUES (actor, lower(TG_OP), TG_TABLE_NAME, row_id, old_row, new_row);

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER charts_audit AFTER INSERT OR UPDATE OR DELETE ON charts
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');
CREATE TRIGGER materials_audit AFTER INSERT OR UPDATE OR DELETE ON materials
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');
CREATE TRIGGER products_audit AFTER INSERT OR UPDATE OR DELETE ON products
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');
CREATE TRIGGER work_orders_audit AFTER INSERT OR UPDATE OR DELETE ON work_orders
    FOR EACH ROW EXECUTE FUNCTION audit_row('id');
CREATE TRIGGER spc_limit_sets_audit AFTER INSERT OR UPDATE OR DELETE ON spc_limit_sets
    FOR EACH ROW EXECUTE FUNCTION audit_row('chart_id');

-- audit_log'un kendisi eklemeli-yalnızdır: güncelleme, silme ve boşaltma yasaktır.
-- +goose StatementBegin
CREATE FUNCTION audit_log_readonly() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only (% is not allowed)', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_change BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_readonly();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_readonly();

-- +goose Down
DROP TRIGGER spc_limit_sets_audit ON spc_limit_sets;
DROP TRIGGER work_orders_audit ON work_orders;
DROP TRIGGER products_audit ON products;
DROP TRIGGER materials_audit ON materials;
DROP TRIGGER charts_audit ON charts;
DROP TABLE audit_log;
DROP FUNCTION audit_log_readonly();
DROP FUNCTION audit_row();