--
-- Test case for GitHub issue #409: internal per-partition FK constraints dumped
--
-- When a foreign key references a partitioned table, PostgreSQL automatically
-- creates one extra pg_constraint row per partition (conparentid != 0) to track
-- the FK on each partition. These are internal artifacts: pg_dump never emits
-- them, only the top-level FK (conparentid = 0) that references the partitioned
-- parent.
--
-- pgschema dump used to emit all of them, producing bogus extra constraints like
-- event_..._fkey1 / event_..._fkey2 that reference individual partitions. This
-- also broke .pgschemaignore'd partitions, leaving dangling REFERENCES.
--

CREATE TABLE session (
    id bigint NOT NULL,
    started_at timestamptz NOT NULL,
    PRIMARY KEY (id, started_at)
) PARTITION BY RANGE (started_at);

CREATE TABLE event (
    session_id bigint NOT NULL,
    session_started_at timestamptz NOT NULL,
    FOREIGN KEY (session_id, session_started_at) REFERENCES session (id, started_at)
);

CREATE TABLE session_2026_01
    PARTITION OF session
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE session_2026_02
    PARTITION OF session
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
