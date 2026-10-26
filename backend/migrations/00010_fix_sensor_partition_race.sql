-- +goose Up
-- İlk yük testinde, ay partisyonu yokken aynı anda gelen isteklerin bir kısmı (3000'de 10)
-- partisyonu ikinci kez oluşturmaya çalışıp hata aldı. Oluşturma artık "zaten var" hatasını yutar.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_sensor_partition(ts TIMESTAMPTZ) RETURNS TEXT AS $$
DECLARE
    month_start    TIMESTAMPTZ := date_trunc('month', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
    partition_name TEXT := 'sensor_readings_' || to_char(date_trunc('month', ts AT TIME ZONE 'UTC'), 'YYYY_MM');
BEGIN
    IF to_regclass('public.' || partition_name) IS NULL THEN
        PERFORM pg_advisory_xact_lock(hashtext('sensor_partitions'));
        BEGIN
            EXECUTE format('CREATE TABLE IF NOT EXISTS public.%I PARTITION OF public.sensor_readings FOR VALUES FROM (%L) TO (%L)',
                partition_name, month_start, month_start + interval '1 month');
        EXCEPTION WHEN duplicate_table THEN
            NULL; -- başka bir işlem az önce oluşturdu
        END;
    END IF;
    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
