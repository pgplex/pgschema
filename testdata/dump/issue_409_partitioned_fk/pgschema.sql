--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.13.3


--
-- Name: session; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session (
    id bigint,
    started_at timestamptz,
    CONSTRAINT session_pkey PRIMARY KEY (id, started_at)
) PARTITION BY RANGE (started_at);

--
-- Name: event; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS event (
    session_id bigint NOT NULL,
    session_started_at timestamptz NOT NULL,
    CONSTRAINT event_session_id_session_started_at_fkey FOREIGN KEY (session_id, session_started_at) REFERENCES session (id, started_at)
);

--
-- Name: session_2026_01; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_01 PARTITION OF session FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00');
DO $pgschema_restore_0$
DECLARE
    index_ids pg_catalog.oid[] := ARRAY[]::pg_catalog.oid[];
    desired_names pg_catalog.text[] := ARRAY[]::pg_catalog.text[];
    found_id pg_catalog.oid;
    current_name pg_catalog.text;
    namespace_name pg_catalog.text;
    namespace_id pg_catalog.oid;
    table_id pg_catalog.oid;
    temporary_name pg_catalog.text;
    position pg_catalog.int4;
    suffix pg_catalog.int4;
BEGIN
    SELECT c.conindid INTO STRICT found_id
    FROM pg_catalog.pg_constraint c
    JOIN pg_catalog.pg_constraint pc ON pc.oid = c.conparentid
    JOIN pg_catalog.pg_class parent_index ON parent_index.oid = pc.conindid
    JOIN pg_catalog.pg_inherits inh ON inh.inhrelid = c.conindid AND inh.inhparent = pc.conindid
    WHERE c.conrelid = E'session_2026_01'::pg_catalog.regclass AND c.contype = E'p'
      AND pc.conrelid = E'session'::pg_catalog.regclass AND pc.conname = E'session_pkey'
      AND parent_index.relname = E'session_pkey';
    index_ids := pg_catalog.array_append(index_ids, found_id);
    desired_names := pg_catalog.array_append(desired_names, E'session_2026_01_pkey');
    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname, n.oid, i.indrelid
        INTO STRICT current_name, namespace_name, namespace_id, table_id
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_catalog.pg_index i ON i.indexrelid = c.oid
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            suffix := 0;
            LOOP
                temporary_name := pg_catalog.format('pgschema_inherited_%s_%s', index_ids[position], suffix);
                EXIT WHEN NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_class
                    WHERE relnamespace = namespace_id AND relname = temporary_name
                ) AND NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_constraint
                    WHERE conrelid = table_id AND conname = temporary_name
                ) AND temporary_name <> ALL(desired_names);
                suffix := suffix + 1;
            END LOOP;
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, temporary_name);
        END IF;
    END LOOP;
    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname INTO STRICT current_name, namespace_name
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, desired_names[position]);
        END IF;
    END LOOP;
END
$pgschema_restore_0$;

--
-- Name: session_2026_02; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_02 PARTITION OF session FOR VALUES FROM ('2026-02-01 00:00:00+00') TO ('2026-03-01 00:00:00+00');
DO $pgschema_restore_0$
DECLARE
    index_ids pg_catalog.oid[] := ARRAY[]::pg_catalog.oid[];
    desired_names pg_catalog.text[] := ARRAY[]::pg_catalog.text[];
    found_id pg_catalog.oid;
    current_name pg_catalog.text;
    namespace_name pg_catalog.text;
    namespace_id pg_catalog.oid;
    table_id pg_catalog.oid;
    temporary_name pg_catalog.text;
    position pg_catalog.int4;
    suffix pg_catalog.int4;
BEGIN
    SELECT c.conindid INTO STRICT found_id
    FROM pg_catalog.pg_constraint c
    JOIN pg_catalog.pg_constraint pc ON pc.oid = c.conparentid
    JOIN pg_catalog.pg_class parent_index ON parent_index.oid = pc.conindid
    JOIN pg_catalog.pg_inherits inh ON inh.inhrelid = c.conindid AND inh.inhparent = pc.conindid
    WHERE c.conrelid = E'session_2026_02'::pg_catalog.regclass AND c.contype = E'p'
      AND pc.conrelid = E'session'::pg_catalog.regclass AND pc.conname = E'session_pkey'
      AND parent_index.relname = E'session_pkey';
    index_ids := pg_catalog.array_append(index_ids, found_id);
    desired_names := pg_catalog.array_append(desired_names, E'session_2026_02_pkey');
    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname, n.oid, i.indrelid
        INTO STRICT current_name, namespace_name, namespace_id, table_id
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_catalog.pg_index i ON i.indexrelid = c.oid
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            suffix := 0;
            LOOP
                temporary_name := pg_catalog.format('pgschema_inherited_%s_%s', index_ids[position], suffix);
                EXIT WHEN NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_class
                    WHERE relnamespace = namespace_id AND relname = temporary_name
                ) AND NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_constraint
                    WHERE conrelid = table_id AND conname = temporary_name
                ) AND temporary_name <> ALL(desired_names);
                suffix := suffix + 1;
            END LOOP;
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, temporary_name);
        END IF;
    END LOOP;
    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname INTO STRICT current_name, namespace_name
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, desired_names[position]);
        END IF;
    END LOOP;
END
$pgschema_restore_0$;

