CREATE TABLE IF NOT EXISTS events (
    id integer NOT NULL,
    region text NOT NULL
) PARTITION BY RANGE (id);

CREATE TABLE IF NOT EXISTS event_notes (
    event_id integer NOT NULL,
    note text
);

CREATE TABLE IF NOT EXISTS events_archive PARTITION OF events FOR VALUES FROM (10) TO (20) PARTITION BY RANGE (id);

CREATE TABLE IF NOT EXISTS events_archive_old PARTITION OF events_archive FOR VALUES FROM (10) TO (15);

CREATE INDEX IF NOT EXISTS events_archive_old_id_idx ON events_archive_old (id);

CREATE TABLE IF NOT EXISTS events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

CREATE INDEX IF NOT EXISTS events_leaf_id_idx ON events_leaf (id);

CREATE TABLE IF NOT EXISTS leaf_notes (
    event_id integer NOT NULL
);

ALTER TABLE ONLY events ADD CONSTRAINT events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY events_archive ADD CONSTRAINT events_archive_pkey PRIMARY KEY (id);

ALTER TABLE ONLY events_archive_old ADD CONSTRAINT events_archive_old_pkey PRIMARY KEY (id);

ALTER INDEX events_archive_pkey ATTACH PARTITION events_archive_old_pkey;

ALTER INDEX events_pkey ATTACH PARTITION events_archive_pkey;

ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_pkey1 PRIMARY KEY (id);

ALTER INDEX events_pkey ATTACH PARTITION events_leaf_pkey1;

ALTER TABLE ONLY events ADD CONSTRAINT events_region_id_key UNIQUE (region, id);

ALTER TABLE ONLY events_archive ADD CONSTRAINT events_archive_region_id_key UNIQUE (region, id);

ALTER TABLE ONLY events_archive_old ADD CONSTRAINT events_archive_old_region_id_key UNIQUE (region, id);

ALTER INDEX events_archive_region_id_key ATTACH PARTITION events_archive_old_region_id_key;

ALTER INDEX events_region_id_key ATTACH PARTITION events_archive_region_id_key;

ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_region_id_key UNIQUE (region, id);

ALTER INDEX events_region_id_key ATTACH PARTITION events_leaf_region_id_key;

CREATE INDEX IF NOT EXISTS events_archive_id_idx ON ONLY events_archive (id);

ALTER INDEX events_archive_id_idx ATTACH PARTITION events_archive_old_id_idx;

CREATE INDEX IF NOT EXISTS events_leaf_pkey ON ONLY events (id);

ALTER INDEX events_leaf_pkey ATTACH PARTITION events_archive_id_idx;

ALTER INDEX events_leaf_pkey ATTACH PARTITION events_leaf_id_idx;

ALTER TABLE event_notes
ADD CONSTRAINT event_notes_event_id_fkey FOREIGN KEY (event_id) REFERENCES events (id);

ALTER TABLE leaf_notes
ADD CONSTRAINT leaf_notes_event_id_fkey FOREIGN KEY (event_id) REFERENCES events_leaf (id);
