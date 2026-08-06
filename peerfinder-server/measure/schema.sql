CREATE TABLE IF NOT EXISTS peers
(
    uuid        TEXT PRIMARY KEY,
    asn         TEXT              NOT NULL,
    id          TEXT              NOT NULL,
    endpoint    TEXT              NOT NULL,
    hmac_key    TEXT              NOT NULL,
    version     TEXT,
    added_at    INTEGER           NOT NULL,
    last_seen   INTEGER,
    last_probed INTEGER,
    disabled    INTEGER DEFAULT 0 NOT NULL,
    UNIQUE (asn, id),
    CHECK (disabled IN (0, 1))
) STRICT;