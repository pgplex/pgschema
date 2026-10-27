CREATE TABLE IF NOT EXISTS events (
    id integer NOT NULL,
    payload text
) PARTITION BY RANGE (id);

ALTER TABLE events ENABLE ROW LEVEL SECURITY;

CREATE TABLE IF NOT EXISTS event_notes (
    event_id integer NOT NULL,
    note text
);

ALTER TABLE event_notes ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION count_events()
RETURNS text
LANGUAGE sql
VOLATILE
BEGIN ATOMIC
 SELECT (count(*))::text AS count
    FROM public.events;
END;

CREATE OR REPLACE FUNCTION count_events_text()
RETURNS text
LANGUAGE sql
VOLATILE
BEGIN ATOMIC
 SELECT public.count_events() AS count_events;
END;

CREATE TABLE IF NOT EXISTS events_leaf PARTITION OF events (
    payload DEFAULT count_events_text()
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
 INSERT INTO public.events (id, payload)
   VALUES (upsert_event.p_id, upsert_event.p_payload) ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = excluded.payload;
END;

CREATE OR REPLACE PROCEDURE upsert_event_proc(
    IN p_id integer,
    IN p_payload text
)
LANGUAGE sql
BEGIN ATOMIC
 INSERT INTO public.events (id, payload)
   VALUES (upsert_event_proc.p_id, upsert_event_proc.p_payload) ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = excluded.payload;
END;

ALTER TABLE event_notes
ADD CONSTRAINT event_notes_event_id_fkey FOREIGN KEY (event_id) REFERENCES events (id);

CREATE POLICY event_notes_grouped ON event_notes TO PUBLIC USING (EXISTS ( SELECT events.payload FROM events GROUP BY events.id));

CREATE POLICY events_grouped ON events TO PUBLIC USING (EXISTS ( SELECT events_1.payload FROM events events_1 GROUP BY events_1.id));
