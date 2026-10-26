-- +goose Up
CREATE TABLE sensor_readings (
    station_id  INTEGER          NOT NULL CHECK (station_id BETWEEN 1 AND 1000),
    recorded_at TIMESTAMPTZ      NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    received_at TIMESTAMPTZ      NOT NULL DEFAULT now()
) PARTITION BY RANGE (recorded_at);

-- Ana tabloda tanımlanan dizin her partisyonda otomatik oluşur.
CREATE INDEX sensor_readings_station_time_idx ON sensor_readings (station_id, recorded_at);

CREATE SCHEMA IF NOT EXISTS archive;

-- Verilen zamanın ayına ait partisyon yoksa oluşturur. Kilit yalnızca oluşturma yolunda alınır,
-- böylece normal yazmalar birbirini beklemez.
-- +goose StatementBegin
CREATE FUNCTION ensure_sensor_partition(ts TIMESTAMPTZ) RETURNS TEXT AS $$
DECLARE
    month_start    TIMESTAMPTZ := date_trunc('month', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
    partition_name TEXT := 'sensor_readings_' || to_char(date_trunc('month', ts AT TIME ZONE 'UTC'), 'YYYY_MM');
BEGIN
    IF to_regclass('public.' || partition_name) IS NULL THEN
        PERFORM pg_advisory_xact_lock(hashtext('sensor_partitions'));
        IF to_regclass('public.' || partition_name) IS NULL THEN
            EXECUTE format('CREATE TABLE public.%I PARTITION OF public.sensor_readings FOR VALUES FROM (%L) TO (%L)',
                partition_name, month_start, month_start + interval '1 month');
        END IF;
    END IF;
    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Üst sınırı cutoff'tan önce ya da eşit olan partisyonları ana tablodan ayırıp archive şemasına taşır.
-- +goose StatementBegin
CREATE FUNCTION archive_sensor_partitions(cutoff TIMESTAMPTZ) RETURNS SETOF TEXT AS $$
DECLARE
    part        RECORD;
    upper_bound TIMESTAMPTZ;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('sensor_partitions'));
    FOR part IN
        SELECT c.relname, pg_get_expr(c.relpartbound, c.oid) AS bound
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        WHERE i.inhparent = 'public.sensor_readings'::regclass
        ORDER BY c.relname
    LOOP
        upper_bound := substring(part.bound from 'TO \(''([^'']+)''\)')::timestamptz;
        IF upper_bound <= cutoff THEN
            EXECUTE format('ALTER TABLE public.sensor_readings DETACH PARTITION public.%I', part.relname);
            EXECUTE format('ALTER TABLE public.%I SET SCHEMA archive', part.relname);
            RETURN NEXT part.relname;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION archive_sensor_partitions(TIMESTAMPTZ);
DROP FUNCTION ensure_sensor_partition(TIMESTAMPTZ);
DROP TABLE sensor_readings;
DROP SCHEMA archive CASCADE;