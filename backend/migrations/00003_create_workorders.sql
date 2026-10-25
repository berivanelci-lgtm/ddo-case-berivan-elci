-- +goose Up
CREATE TABLE products (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE materials (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    stock_qty  BIGINT      NOT NULL CHECK (stock_qty >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bom_lines (
    product_id   BIGINT NOT NULL REFERENCES products (id),
    material_id  BIGINT NOT NULL REFERENCES materials (id),
    qty_per_unit BIGINT NOT NULL CHECK (qty_per_unit > 0),
    PRIMARY KEY (product_id, material_id)
);

CREATE TABLE work_orders (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id     BIGINT      NOT NULL REFERENCES products (id),
    planned_qty    BIGINT      NOT NULL CHECK (planned_qty > 0),
    status         TEXT        NOT NULL CHECK (status IN (
        'planned', 'released', 'in_progress', 'completed', 'cancelled'
    )),
    started_qty    BIGINT      NOT NULL DEFAULT 0 CHECK (started_qty >= 0),
    good_qty       BIGINT      NOT NULL DEFAULT 0 CHECK (good_qty >= 0),
    scrap_qty      BIGINT      NOT NULL DEFAULT 0 CHECK (scrap_qty >= 0),
    rework_pending BIGINT      NOT NULL DEFAULT 0 CHECK (rework_pending >= 0),
    created_by     BIGINT      NOT NULL REFERENCES users (id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (started_qty = good_qty + scrap_qty + rework_pending)
);

CREATE INDEX work_orders_product_idx ON work_orders (product_id);

CREATE TABLE production_records (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    work_order_id BIGINT      NOT NULL REFERENCES work_orders (id),
    kind          TEXT        NOT NULL CHECK (kind IN ('production', 'rework')),
    good_qty      BIGINT      NOT NULL CHECK (good_qty >= 0),
    scrap_qty     BIGINT      NOT NULL CHECK (scrap_qty >= 0),
    rework_qty    BIGINT      NOT NULL CHECK (rework_qty >= 0),
    created_by    BIGINT      NOT NULL REFERENCES users (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX production_records_order_idx ON production_records (work_order_id, id);

CREATE TABLE material_consumptions (
    work_order_id BIGINT NOT NULL REFERENCES work_orders (id),
    material_id   BIGINT NOT NULL REFERENCES materials (id),
    quantity      BIGINT NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (work_order_id, material_id)
);

-- +goose Down
DROP TABLE material_consumptions;
DROP TABLE production_records;
DROP TABLE work_orders;
DROP TABLE bom_lines;
DROP TABLE materials;
DROP TABLE products;