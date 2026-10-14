--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.13.3


--
-- Name: ledger; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger (
    id uuid,
    amount bigint NOT NULL,
    ts timestamptz,
    CONSTRAINT ledger_pkey PRIMARY KEY (ts, id)
) PARTITION BY RANGE (ts);

--
-- Name: ledger_2026_06; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger_2026_06 PARTITION OF ledger FOR VALUES FROM ('2026-06-01 00:00:00+00') TO ('2026-07-01 00:00:00+00');
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
    WHERE c.conrelid = E'ledger_2026_06'::pg_catalog.regclass AND c.contype = E'p'
      AND pc.conrelid = E'ledger'::pg_catalog.regclass AND pc.conname = E'ledger_pkey'
      AND parent_index.relname = E'ledger_pkey';
    index_ids := pg_catalog.array_append(index_ids, found_id);
    desired_names := pg_catalog.array_append(desired_names, E'ledger_2026_06_pkey');
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
-- Name: tg_noop(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION tg_noop()
RETURNS trigger
LANGUAGE plpgsql
VOLATILE
AS $$ BEGIN RETURN NEW; END
$$;

--
-- Name: trg_rollup; Type: TRIGGER; Schema: -; Owner: -
--

CREATE OR REPLACE TRIGGER trg_rollup
    AFTER INSERT ON ledger
    FOR EACH ROW
    EXECUTE FUNCTION tg_noop();

