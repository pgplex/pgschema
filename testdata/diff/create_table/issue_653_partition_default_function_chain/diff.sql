CREATE TABLE IF NOT EXISTS events (
    id integer NOT NULL,
    payload text
) PARTITION BY RANGE (id);

ALTER TABLE events ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION count_events()
RETURNS text
LANGUAGE sql
VOLATILE
BEGIN ATOMIC
 SELECT (count(*))::text AS count
    FROM events;
END;

CREATE TABLE IF NOT EXISTS events_leaf PARTITION OF events (
    payload DEFAULT count_events()
) FOR VALUES FROM (0) TO (10);

ALTER TABLE ONLY events ADD CONSTRAINT events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_pkey PRIMARY KEY (id);

ALTER INDEX events_pkey ATTACH PARTITION events_leaf_pkey;

CREATE OR REPLACE FUNCTION upsert_event(
    p_id integer,
    p_payload text
)
RETURNS void
LANGUAGE sql
VOLATILE
BEGIN ATOMIC
 INSERT INTO events (id, payload)
   VALUES (upsert_event.p_id, upsert_event.p_payload) ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = excluded.payload;
END;

CREATE POLICY events_grouped ON events TO PUBLIC USING (EXISTS ( SELECT events_1.payload FROM events events_1 GROUP BY events_1.id));
