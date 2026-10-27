--
-- Test case for GitHub issue #653: inherited constraint index names collide
-- with standalone indexes on replay
--
-- PARTITION OF copies the parent's PRIMARY KEY and UNIQUE constraints onto
-- the new partition under generated names. When a standalone index already
-- holds such a name, PostgreSQL picks the next free one (events_leaf_pkey1).
-- A dump that creates the partition with the constraint inline, before the
-- standalone index, hands the generated name to the copy; the later
-- CREATE INDEX IF NOT EXISTS is skipped and ATTACH PARTITION fails.
--
-- The dump therefore adds these constraints after the tables with
-- ALTER TABLE ONLY, each partition copy under its catalog name, and attaches
-- the copies explicitly.
--

CREATE TABLE events (
    id integer PRIMARY KEY,
    region text NOT NULL,
    CONSTRAINT events_region_id_key UNIQUE (region, id)
) PARTITION BY RANGE (id);

CREATE INDEX events_leaf_pkey ON events (id);

CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

CREATE TABLE events_archive PARTITION OF events
    FOR VALUES FROM (10) TO (20) PARTITION BY RANGE (id);

CREATE TABLE events_archive_old PARTITION OF events_archive FOR VALUES FROM (10) TO (15);

-- A renamed copy whose original name now belongs to an index on another
-- column. Replaying with the constraint inline silently dropped this index.
ALTER INDEX events_archive_old_pkey RENAME TO events_archive_old_pk;
CREATE INDEX events_archive_old_pkey ON events_archive_old (region);

CREATE TABLE event_notes (
    event_id integer NOT NULL REFERENCES events (id),
    note text
);
