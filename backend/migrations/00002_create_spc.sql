-- +goose Up
CREATE TABLE charts (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL UNIQUE,
    chart_type    TEXT        NOT NULL CHECK (chart_type IN ('individual', 'xbar_r')),
    subgroup_size INTEGER     NOT NULL,
    created_by    BIGINT      NOT NULL REFERENCES users (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (chart_type = 'individual' AND subgroup_size = 1)
        OR (chart_type = 'xbar_r' AND subgroup_size BETWEEN 2 AND 10)
    )
);

CREATE TABLE spc_points (
    chart_id   BIGINT             NOT NULL REFERENCES charts (id),
    seq        BIGINT             NOT NULL CHECK (seq > 0),
    subgroup   DOUBLE PRECISION[] NOT NULL CHECK (cardinality(subgroup) > 0),
    statistic  DOUBLE PRECISION   NOT NULL,
    created_at TIMESTAMPTZ        NOT NULL DEFAULT now(),
    PRIMARY KEY (chart_id, seq)
);

CREATE TABLE spc_limit_sets (
    chart_id     BIGINT           NOT NULL REFERENCES charts (id),
    version      INTEGER          NOT NULL CHECK (version > 0),
    center       DOUBLE PRECISION NOT NULL,
    sigma        DOUBLE PRECISION NOT NULL CHECK (sigma > 0),
    ucl          DOUBLE PRECISION NOT NULL,
    lcl          DOUBLE PRECISION NOT NULL,
    range_center DOUBLE PRECISION,
    range_ucl    DOUBLE PRECISION,
    range_lcl    DOUBLE PRECISION,
    created_at   TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (chart_id, version),
    CHECK (
        (range_center IS NULL) = (range_ucl IS NULL)
        AND (range_center IS NULL) = (range_lcl IS NULL)
    )
);

CREATE TABLE spc_alarms (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chart_id      BIGINT           NOT NULL,
    point_seq     BIGINT           NOT NULL,
    rule          TEXT             NOT NULL CHECK (rule IN (
        'beyond_3_sigma',
        'same_side_8',
        'trend_6',
        'two_of_three_beyond_2_sigma',
        'four_of_five_beyond_1_sigma'
    )),
    point_value   DOUBLE PRECISION NOT NULL,
    limit_version INTEGER          NOT NULL,
    created_at    TIMESTAMPTZ      NOT NULL DEFAULT now(),
    UNIQUE (chart_id, point_seq, rule),
    FOREIGN KEY (chart_id, point_seq) REFERENCES spc_points (chart_id, seq),
    FOREIGN KEY (chart_id, limit_version) REFERENCES spc_limit_sets (chart_id, version)
);

CREATE INDEX spc_alarms_chart_recent_idx ON spc_alarms (chart_id, id DESC);

-- +goose Down
DROP TABLE spc_alarms;
DROP TABLE spc_limit_sets;
DROP TABLE spc_points;
DROP TABLE charts;