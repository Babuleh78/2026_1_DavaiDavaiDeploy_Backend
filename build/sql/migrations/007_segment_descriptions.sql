CREATE TABLE IF NOT EXISTS segment_descriptions (
    dance_id        TEXT        NOT NULL,
    segment_index   INT         NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dance_id, segment_index)
);

CREATE INDEX IF NOT EXISTS idx_segment_descriptions_dance
    ON segment_descriptions(dance_id);
