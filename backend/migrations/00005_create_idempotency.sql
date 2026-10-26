-- +goose Up
CREATE TABLE idempotency_keys (
    user_id       BIGINT      NOT NULL REFERENCES users (id),
    key           TEXT        NOT NULL CHECK (char_length(key) BETWEEN 1 AND 128),
    request_hash  TEXT        NOT NULL,
    status_code   INTEGER     NOT NULL CHECK (status_code BETWEEN 200 AND 299),
    response_body JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE idempotency_keys;