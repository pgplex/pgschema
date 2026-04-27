-- GetSchemas retrieves all user-defined schemas
-- name: GetSchemas :many
SELECT 
    schema_name
FROM information_schema.schemata
WHERE 
    schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND schema_name NOT LIKE 'pg_temp_%'
    AND schema_name NOT LIKE 'pg_toast_temp_%'
ORDER BY schema_name;

-- GetSchema retrieves a specific schema by name
-- name: GetSchema :one
SELECT 
    schema_name
FROM information_schema.schemata
WHERE 
    schema_name = $1
    AND schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND schema_name NOT LIKE 'pg_temp_%'
    AND schema_name NOT LIKE 'pg_toast_temp_%';

-- GetTables retrieves all tables in the database with metadata
-- name: GetTables :many
SELECT 
    t.table_schema,
    t.table_name,
    t.table_type,
    COALESCE(d.description, '') AS table_comment
FROM information_schema.tables t
LEFT JOIN pg_namespace n ON n.nspname = t.table_schema
LEFT JOIN pg_class c ON c.relname = t.table_name AND c.relnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = c.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = 0
WHERE
    t.table_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND t.table_schema NOT LIKE 'pg_temp_%'
    AND t.table_schema NOT LIKE 'pg_toast_temp_%'
    AND t.table_type IN ('BASE TABLE', 'VIEW')
ORDER BY t.table_schema, t.table_name;

-- GetTablesForSchema retrieves all tables in a specific schema with metadata
-- name: GetTablesForSchema :many
SELECT
    t.table_schema,
    t.table_name,
    t.table_type,
    COALESCE(d.description, '') AS table_comment
FROM information_schema.tables t
LEFT JOIN pg_namespace n ON n.nspname = t.table_schema
LEFT JOIN pg_class c ON c.relname = t.table_name AND c.relnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = c.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = 0
WHERE
    t.table_schema = $1
    AND t.table_type IN ('BASE TABLE', 'VIEW')
ORDER BY t.table_name;

-- GetColumns retrieves all columns for all tables
-- name: GetColumns :many
WITH column_base AS (
    SELECT
        c.table_schema,
        c.table_name,
        c.column_name,
        c.ordinal_position,
        c.column_default,
        c.is_nullable,
        c.data_type,
        c.character_maximum_length,
        c.numeric_precision,
        c.numeric_scale,
        c.udt_name,
        COALESCE(d.description, '') AS column_comment,
        CASE
            WHEN dt.typtype = 'd' THEN
                CASE WHEN dn.nspname = c.table_schema THEN dt.typname
                     ELSE dn.nspname || '.' || dt.typname
                END
            WHEN dt.typtype = 'e' OR dt.typtype = 'c' THEN
                CASE WHEN dn.nspname = c.table_schema THEN dt.typname
                     ELSE dn.nspname || '.' || dt.typname
                END
            WHEN dt.typtype = 'b' AND dt.typcategory = 'A' THEN
                -- Array types: apply same schema qualification logic to element type
                -- Use typcategory = 'A' rather than typelem <> 0; the latter is true
                -- for non-array fixed-length types like name (typelem points to char).
                CASE
                    WHEN en.nspname = 'pg_catalog' THEN et.typname || '[]'
                    WHEN en.nspname = c.table_schema THEN et.typname || '[]'
                    ELSE en.nspname || '.' || et.typname || '[]'
                END
            WHEN dt.typtype = 'b' THEN
                -- Non-array base types: qualify if not in pg_catalog or table's schema
                -- Use format_type to preserve typmod for extension types (e.g., vector(384) for pgvector)
                CASE
                    WHEN dn.nspname = 'pg_catalog' THEN c.udt_name
                    WHEN dn.nspname = c.table_schema THEN
                        dt.typname || COALESCE(substring(format_type(a.atttypid, a.atttypmod) FROM '\([^)]*\)'), '')
                    ELSE
                        dn.nspname || '.' || dt.typname || COALESCE(substring(format_type(a.atttypid, a.atttypmod) FROM '\([^)]*\)'), '')
                END
            ELSE c.udt_name
        END AS resolved_type,
        c.is_identity,
        c.identity_generation,
        c.identity_start,
        c.identity_increment,
        c.identity_maximum,
        c.identity_minimum,
        c.identity_cycle,
        a.attgenerated,
        ad.adbin,
        ad.adrelid
    FROM information_schema.columns c
    LEFT JOIN pg_namespace n ON n.nspname = c.table_schema
    LEFT JOIN pg_class cl ON cl.relname = c.table_name AND cl.relnamespace = n.oid
    LEFT JOIN pg_description d ON d.objoid = cl.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = c.ordinal_position
    LEFT JOIN pg_attribute a ON a.attrelid = cl.oid AND a.attname = c.column_name
    LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
    LEFT JOIN pg_type dt ON dt.oid = a.atttypid
    LEFT JOIN pg_namespace dn ON dt.typnamespace = dn.oid
    LEFT JOIN pg_type et ON dt.typelem = et.oid
    LEFT JOIN pg_namespace en ON et.typnamespace = en.oid
    WHERE
        c.table_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
        AND c.table_schema NOT LIKE 'pg_temp_%'
        AND c.table_schema NOT LIKE 'pg_toast_temp_%'
)
SELECT
    cb.table_schema,
    cb.table_name,
    cb.column_name,
    cb.ordinal_position,
    -- Use the column_default from LATERAL join which has proper search_path set
    ge.column_default,
    cb.is_nullable,
    cb.data_type,
    cb.character_maximum_length,
    cb.numeric_precision,
    cb.numeric_scale,
    cb.udt_name,
    cb.column_comment,
    cb.resolved_type,
    cb.is_identity,
    cb.identity_generation,
    cb.identity_start,
    cb.identity_increment,
    cb.identity_maximum,
    cb.identity_minimum,
    cb.identity_cycle,
    cb.attgenerated,
    -- Use LATERAL join to guarantee execution order:
    -- 1. set_config sets search_path to only the table's schema
    -- 2. pg_get_expr then uses that search_path
    -- This ensures cross-schema type references in column defaults and generated columns
    -- are properly qualified (Issue #218)
    ge.generated_expr
FROM column_base cb
LEFT JOIN LATERAL (
    SELECT
        -- Set search_path to only pg_catalog to force pg_get_expr to include schema qualifiers
        -- for all user-defined types and functions. The normalization code will then strip
        -- same-schema function qualifiers while preserving type qualifiers (Issue #218)
        set_config('search_path', 'pg_catalog', true) as dummy,
        CASE
            WHEN cb.attgenerated = 's' THEN NULL  -- Generated columns don't have defaults
            ELSE COALESCE(pg_get_expr(cb.adbin, cb.adrelid), cb.column_default)
        END as column_default,
        CASE
            WHEN cb.attgenerated = 's' THEN pg_get_expr(cb.adbin, cb.adrelid)
            ELSE NULL
        END as generated_expr
) ge ON true
ORDER BY cb.table_schema, cb.table_name, cb.ordinal_position;

-- GetColumnsForSchema retrieves all columns for tables in a specific schema
-- name: GetColumnsForSchema :many
WITH column_base AS (
    SELECT
        c.table_schema,
        c.table_name,
        c.column_name,
        c.ordinal_position,
        c.column_default,
        c.is_nullable,
        c.data_type,
        c.character_maximum_length,
        c.numeric_precision,
        c.numeric_scale,
        c.udt_name,
        COALESCE(d.description, '') AS column_comment,
        CASE
            WHEN dt.typtype = 'd' THEN
                CASE WHEN dn.nspname = c.table_schema THEN dt.typname
                     ELSE dn.nspname || '.' || dt.typname
                END
            WHEN dt.typtype = 'e' OR dt.typtype = 'c' THEN
                CASE WHEN dn.nspname = c.table_schema THEN dt.typname
                     ELSE dn.nspname || '.' || dt.typname
                END
            WHEN dt.typtype = 'b' AND dt.typcategory = 'A' THEN
                -- Array types: apply same schema qualification logic to element type
                -- Use typcategory = 'A' rather than typelem <> 0; the latter is true
                -- for non-array fixed-length types like name (typelem points to char).
                CASE
                    WHEN en.nspname = 'pg_catalog' THEN et.typname || '[]'
                    WHEN en.nspname = c.table_schema THEN et.typname || '[]'
                    ELSE en.nspname || '.' || et.typname || '[]'
                END
            WHEN dt.typtype = 'b' THEN
                -- Non-array base types: qualify if not in pg_catalog or table's schema
                -- Use format_type to preserve typmod for extension types (e.g., vector(384) for pgvector)
                CASE
                    WHEN dn.nspname = 'pg_catalog' THEN c.udt_name
                    WHEN dn.nspname = c.table_schema THEN
                        dt.typname || COALESCE(substring(format_type(a.atttypid, a.atttypmod) FROM '\([^)]*\)'), '')
                    ELSE
                        dn.nspname || '.' || dt.typname || COALESCE(substring(format_type(a.atttypid, a.atttypmod) FROM '\([^)]*\)'), '')
                END
            ELSE c.udt_name
        END AS resolved_type,
        c.is_identity,
        c.identity_generation,
        c.identity_start,
        c.identity_increment,
        c.identity_maximum,
        c.identity_minimum,
        c.identity_cycle,
        a.attgenerated,
        ad.adbin,
        ad.adrelid,
        cl.oid AS table_oid
    FROM information_schema.columns c
    LEFT JOIN pg_namespace n ON n.nspname = c.table_schema
    LEFT JOIN pg_class cl ON cl.relname = c.table_name AND cl.relnamespace = n.oid
    LEFT JOIN pg_description d ON d.objoid = cl.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = c.ordinal_position
    LEFT JOIN pg_attribute a ON a.attrelid = cl.oid AND a.attname = c.column_name
    LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
    LEFT JOIN pg_type dt ON dt.oid = a.atttypid
    LEFT JOIN pg_namespace dn ON dt.typnamespace = dn.oid
    LEFT JOIN pg_type et ON dt.typelem = et.oid
    LEFT JOIN pg_namespace en ON et.typnamespace = en.oid
    WHERE
        c.table_schema = $1
)
SELECT
    cb.table_schema,
    cb.table_name,
    cb.column_name,
    cb.ordinal_position,
    -- Use the column_default from LATERAL join which has proper search_path set
    ge.column_default,
    cb.is_nullable,
    cb.data_type,
    cb.character_maximum_length,
    cb.numeric_precision,
    cb.numeric_scale,
    cb.udt_name,
    cb.column_comment,
    cb.resolved_type,
    cb.is_identity,
    cb.identity_generation,
    cb.identity_start,
    cb.identity_increment,
    cb.identity_maximum,
    cb.identity_minimum,
    cb.identity_cycle,
    cb.attgenerated,
    -- Use LATERAL join to guarantee execution order:
    -- 1. set_config sets search_path to only pg_catalog
    -- 2. pg_get_expr then uses that search_path and includes schema qualifiers for user types
    -- This ensures type references in column defaults and generated columns are properly
    -- qualified (Issue #218). The normalization code strips same-schema function qualifiers.
    --
    -- NOTE: The 'dummy' column in the LATERAL subquery forces set_config to execute
    -- before pg_get_expr. PostgreSQL evaluates SELECT columns left-to-right within
    -- a single query level. The LATERAL join guarantees this happens row-by-row,
    -- and 'ON true' in the join condition ensures the LATERAL subquery executes for every row.
    -- This pattern mirrors GetViewsForSchema (line 959-963) for consistency.
    --
    -- Alternative considered: Create a custom PostgreSQL function wrapping pg_get_expr
    -- with search_path control. Rejected because:
    -- 1. Requires creating database objects (function) on target database
    -- 2. pgschema operates in read-only inspection mode
    -- 3. LATERAL join pattern is PostgreSQL-native and well-documented
    ge.generated_expr
FROM column_base cb
LEFT JOIN LATERAL (
    SELECT
        -- Set search_path to only pg_catalog to force pg_get_expr to include schema qualifiers
        -- for all user-defined types and functions. The normalization code will then strip
        -- same-schema function qualifiers while preserving type qualifiers (Issue #218)
        set_config('search_path', 'pg_catalog', true) as dummy,
        CASE
            WHEN cb.attgenerated = 's' THEN NULL  -- Generated columns don't have defaults
            ELSE COALESCE(pg_get_expr(cb.adbin, cb.adrelid), cb.column_default)
        END as column_default,
        CASE
            WHEN cb.attgenerated = 's' THEN pg_get_expr(cb.adbin, cb.adrelid)
            ELSE NULL
        END as generated_expr
) ge ON true
ORDER BY cb.table_name, cb.ordinal_position;

-- GetConstraints retrieves all table constraints  
-- name: GetConstraints :many
SELECT 
    n.nspname AS table_schema,
    cl.relname AS table_name,
    c.conname AS constraint_name,
    CASE c.contype
        WHEN 'c' THEN 'CHECK'
        WHEN 'f' THEN 'FOREIGN KEY'
        WHEN 'p' THEN 'PRIMARY KEY'
        WHEN 'u' THEN 'UNIQUE'
        WHEN 'x' THEN 'EXCLUDE'
        ELSE 'UNKNOWN'
    END AS constraint_type,
    COALESCE(a.attname, '') AS column_name,
    COALESCE(a.attnum, 0) AS ordinal_position,
    COALESCE(fn.nspname, '') AS foreign_table_schema,
    COALESCE(fcl.relname, '') AS foreign_table_name,
    COALESCE(fa.attname, '') AS foreign_column_name,
    COALESCE(fa.attnum, 0) AS foreign_ordinal_position,
    CASE WHEN c.contype = 'c' THEN pg_get_constraintdef(c.oid, true) ELSE NULL END AS check_clause,
    CASE WHEN c.contype = 'x' THEN pg_get_constraintdef(c.oid, true) ELSE NULL END AS exclusion_definition,
    CASE c.confdeltype
        WHEN 'a' THEN 'NO ACTION'
        WHEN 'r' THEN 'RESTRICT'
        WHEN 'c' THEN 'CASCADE'
        WHEN 'n' THEN 'SET NULL'
        WHEN 'd' THEN 'SET DEFAULT'
        ELSE NULL
    END AS delete_rule,
    CASE c.confupdtype
        WHEN 'a' THEN 'NO ACTION'
        WHEN 'r' THEN 'RESTRICT'
        WHEN 'c' THEN 'CASCADE'
        WHEN 'n' THEN 'SET NULL'
        WHEN 'd' THEN 'SET DEFAULT'
        ELSE NULL
    END AS update_rule,
    c.condeferrable AS deferrable,
    c.condeferred AS initially_deferred,
    c.convalidated AS is_valid,
    COALESCE((to_jsonb(c) ->> 'conperiod')::boolean, false) AS is_period,
    c.connoinherit AS no_inherit
FROM pg_constraint c
JOIN pg_class cl ON c.conrelid = cl.oid
JOIN pg_namespace n ON cl.relnamespace = n.oid
LEFT JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
LEFT JOIN pg_class fcl ON c.confrelid = fcl.oid
LEFT JOIN pg_namespace fn ON fcl.relnamespace = fn.oid
LEFT JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = c.confkey[array_position(c.conkey, a.attnum)]
WHERE n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, cl.relname, c.contype, c.conname, a.attnum;

-- GetIndexes retrieves all indexes including regular and unique indexes created with CREATE INDEX
-- IMPORTANT: Uses LATERAL join with set_config to temporarily set search_path to empty
-- This ensures pg_get_expr() includes schema qualifiers for types in partial index predicates,
-- matching pg_dump's behavior and preventing false positives when comparing schemas
-- name: GetIndexes :many
WITH index_base AS (
    SELECT
        n.nspname as schemaname,
        t.relname as tablename,
        i.relname as indexname,
        idx.indisunique as is_unique,
        idx.indisprimary as is_primary,
        (idx.indpred IS NOT NULL) as is_partial,
        am.amname as method,
        pg_get_indexdef(idx.indexrelid) as indexdef,
        idx.indpred,
        idx.indrelid,
        CASE
            WHEN idx.indexprs IS NOT NULL THEN true
            ELSE false
        END as has_expressions
    FROM pg_index idx
    JOIN pg_class i ON i.oid = idx.indexrelid
    JOIN pg_class t ON t.oid = idx.indrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    JOIN pg_am am ON am.oid = i.relam
    WHERE
        NOT idx.indisprimary
        AND NOT EXISTS (
            SELECT 1 FROM pg_constraint c
            WHERE c.conindid = idx.indexrelid
            AND c.contype IN ('u', 'p', 'x')
        )
        AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
        AND n.nspname NOT LIKE 'pg_temp_%'
        AND n.nspname NOT LIKE 'pg_toast_temp_%'
)
SELECT
    ib.schemaname,
    ib.tablename,
    ib.indexname,
    ib.is_unique,
    ib.is_primary,
    ib.is_partial,
    ib.method,
    ib.indexdef,
    -- Use LATERAL join to guarantee execution order:
    -- 1. set_config sets search_path to empty (like pg_dump does)
    -- 2. pg_get_expr then uses that search_path
    -- This ensures type references are schema-qualified (e.g., 'value'::public.my_enum)
    sp.partial_predicate,
    ib.has_expressions
FROM index_base ib
CROSS JOIN LATERAL (
    SELECT
        set_config('search_path', '', true) as dummy,
        CASE
            WHEN ib.indpred IS NOT NULL THEN pg_get_expr(ib.indpred, ib.indrelid)
            ELSE NULL
        END as partial_predicate
) sp
ORDER BY ib.schemaname, ib.tablename, ib.indexname;

-- GetIndexesForSchema retrieves all indexes for a specific schema
-- IMPORTANT: Uses LATERAL join with set_config to temporarily set search_path to empty
-- This ensures pg_get_expr() includes schema qualifiers for types in partial index predicates,
-- matching pg_dump's behavior and preventing false positives when comparing schemas
-- name: GetIndexesForSchema :many
WITH index_base AS (
    SELECT
        n.nspname as schemaname,
        t.relname as tablename,
        i.relname as indexname,
        idx.indisunique as is_unique,
        idx.indisprimary as is_primary,
        (idx.indpred IS NOT NULL) as is_partial,
        am.amname as method,
        pg_get_indexdef(idx.indexrelid) as indexdef,
        idx.indpred,
        idx.indrelid,
        CASE
            WHEN idx.indexprs IS NOT NULL THEN true
            ELSE false
        END as has_expressions,
        COALESCE(d.description, '') AS index_comment,
        idx.indnkeyatts as num_key_columns,
        idx.indnatts as num_columns,
        ARRAY(
            SELECT pg_get_indexdef(idx.indexrelid, k::int, true)
            FROM generate_series(1, idx.indnkeyatts) k
        ) as column_definitions,
        ARRAY(
            SELECT
                CASE
                    WHEN (idx.indoption[k-1] & 1) = 1 THEN 'DESC'
                    ELSE 'ASC'
                END
            FROM generate_series(1, idx.indnkeyatts) k
        ) as column_directions,
        ARRAY(
            SELECT CASE
                WHEN opc.opcdefault THEN ''  -- Omit default operator classes
                ELSE COALESCE(opc.opcname, '')
            END
            FROM generate_series(1, idx.indnkeyatts) k
            LEFT JOIN pg_opclass opc ON opc.oid = idx.indclass[k-1]
        ) as column_opclasses,
        ARRAY(
            SELECT pg_get_indexdef(idx.indexrelid, k::int, true)
            FROM generate_series(idx.indnkeyatts + 1, idx.indnatts) k
        ) as include_columns
    FROM pg_index idx
    JOIN pg_class i ON i.oid = idx.indexrelid
    JOIN pg_class t ON t.oid = idx.indrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    JOIN pg_am am ON am.oid = i.relam
    LEFT JOIN pg_description d ON d.objoid = i.oid AND d.objsubid = 0
    WHERE
        NOT idx.indisprimary
        AND NOT EXISTS (
            SELECT 1 FROM pg_constraint c
            WHERE c.conindid = idx.indexrelid
            AND c.contype IN ('u', 'p', 'x')
        )
        AND n.nspname = $1
)
SELECT
    ib.schemaname,
    ib.tablename,
    ib.indexname,
    ib.is_unique,
    ib.is_primary,
    ib.is_partial,
    ib.method,
    ib.indexdef,
    -- Use LATERAL join to guarantee execution order:
    -- 1. set_config sets search_path to empty (like pg_dump does)
    -- 2. pg_get_expr then uses that search_path
    -- This ensures type references are schema-qualified (e.g., 'value'::public.my_enum)
    sp.partial_predicate,
    ib.has_expressions,
    ib.index_comment,
    ib.num_key_columns,
    ib.num_columns,
    ib.column_definitions,
    ib.column_directions,
    ib.column_opclasses,
    ib.include_columns
FROM index_base ib
CROSS JOIN LATERAL (
    SELECT
        set_config('search_path', '', true) as dummy,
        CASE
            WHEN ib.indpred IS NOT NULL THEN pg_get_expr(ib.indpred, ib.indrelid)
            ELSE NULL
        END as partial_predicate
) sp
ORDER BY ib.schemaname, ib.tablename, ib.indexname;

-- GetSequences retrieves all sequences
-- name: GetSequences :many
SELECT 
    sequence_schema,
    sequence_name,
    data_type,
    start_value,
    minimum_value,
    maximum_value,
    increment,
    cycle_option
FROM information_schema.sequences
WHERE 
    sequence_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND sequence_schema NOT LIKE 'pg_temp_%'
    AND sequence_schema NOT LIKE 'pg_toast_temp_%'
ORDER BY sequence_schema, sequence_name;

-- GetFunctions retrieves all user-defined functions (excluding extension members)
-- name: GetFunctions :many
SELECT
    r.routine_schema,
    r.routine_name,
    p.prosrc AS routine_definition,
    r.routine_type,
    COALESCE(pg_get_function_result(p.oid), r.data_type) AS data_type,
    r.external_language,
    COALESCE(desc_func.description, '') AS function_comment,
    oidvectortypes(p.proargtypes) AS function_arguments,
    pg_get_function_arguments(p.oid) AS function_signature,
    CASE p.provolatile
        WHEN 'i' THEN 'IMMUTABLE'
        WHEN 's' THEN 'STABLE'
        WHEN 'v' THEN 'VOLATILE'
        ELSE NULL
    END AS volatility,
    p.proisstrict AS is_strict,
    p.prosecdef AS is_security_definer
FROM information_schema.routines r
LEFT JOIN pg_proc p ON p.proname = r.routine_name
    AND p.pronamespace = (SELECT oid FROM pg_namespace WHERE nspname = r.routine_schema)
    AND p.oid = (regexp_match(r.specific_name, '_(\d+)$'))[1]::oid
LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
LEFT JOIN pg_description desc_func ON desc_func.objoid = p.oid AND desc_func.classoid = 'pg_proc'::regclass
WHERE
    r.routine_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND r.routine_schema NOT LIKE 'pg_temp_%'
    AND r.routine_schema NOT LIKE 'pg_toast_temp_%'
    AND r.routine_type = 'FUNCTION'
    AND d.objid IS NULL  -- Exclude functions that are extension members
ORDER BY r.routine_schema, r.routine_name;

-- GetProcedures retrieves all user-defined procedures (excluding extension members)
-- name: GetProcedures :many
SELECT
    r.routine_schema,
    r.routine_name,
    p.prosrc AS routine_definition,
    r.routine_type,
    r.external_language,
    COALESCE(desc_proc.description, '') AS procedure_comment,
    oidvectortypes(p.proargtypes) AS procedure_arguments,
    pg_get_function_arguments(p.oid) AS procedure_signature
FROM information_schema.routines r
LEFT JOIN pg_proc p ON p.proname = r.routine_name
    AND p.pronamespace = (SELECT oid FROM pg_namespace WHERE nspname = r.routine_schema)
    AND p.oid = (regexp_match(r.specific_name, '_(\d+)$'))[1]::oid
LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
LEFT JOIN pg_description desc_proc ON desc_proc.objoid = p.oid AND desc_proc.classoid = 'pg_proc'::regclass
WHERE
    r.routine_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND r.routine_schema NOT LIKE 'pg_temp_%'
    AND r.routine_schema NOT LIKE 'pg_toast_temp_%'
    AND r.routine_type = 'PROCEDURE'
    AND d.objid IS NULL  -- Exclude procedures that are extension members
ORDER BY r.routine_schema, r.routine_name;

-- GetAggregates retrieves all user-defined aggregates
-- name: GetAggregates :many
SELECT 
    n.nspname AS aggregate_schema,
    p.proname AS aggregate_name,
    pg_get_function_arguments(p.oid) AS aggregate_signature,
    oidvectortypes(p.proargtypes) AS aggregate_arguments,
    format_type(p.prorettype, NULL) AS aggregate_return_type,
    -- Get transition function
    COALESCE(tf.proname, '') AS transition_function,
    COALESCE(tfn.nspname, '') AS transition_function_schema,
    -- Get state type
    format_type(a.aggtranstype, NULL) AS state_type,
    -- Get initial condition
    a.agginitval AS initial_condition,
    -- Get final function if exists
    COALESCE(ff.proname, '') AS final_function,
    COALESCE(ffn.nspname, '') AS final_function_schema,
    -- Comment
    COALESCE(d.description, '') AS aggregate_comment
FROM pg_proc p
JOIN pg_namespace n ON p.pronamespace = n.oid
JOIN pg_aggregate a ON a.aggfnoid = p.oid
LEFT JOIN pg_proc tf ON a.aggtransfn = tf.oid
LEFT JOIN pg_namespace tfn ON tf.pronamespace = tfn.oid
LEFT JOIN pg_proc ff ON a.aggfinalfn = ff.oid
LEFT JOIN pg_namespace ffn ON ff.pronamespace = ffn.oid
LEFT JOIN pg_description d ON d.objoid = p.oid AND d.classoid = 'pg_proc'::regclass
WHERE p.prokind = 'a'  -- Only aggregates
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND NOT EXISTS (
        SELECT 1 FROM pg_depend dep 
        WHERE dep.objid = p.oid AND dep.deptype = 'e'
    )  -- Exclude extension members
ORDER BY n.nspname, p.proname;

-- GetViews retrieves all views and materialized views
-- name: GetViews :many
SELECT 
    n.nspname AS table_schema,
    c.relname AS table_name,
    pg_get_viewdef(c.oid, true) AS view_definition,
    COALESCE(d.description, '') AS view_comment,
    (c.relkind = 'm') AS is_materialized
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = c.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = 0
WHERE 
    c.relkind IN ('v', 'm') -- views and materialized views
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, c.relname;


-- GetTypes retrieves all user-defined types (ENUM and composite types)
-- name: GetTypes :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    CASE t.typtype
        WHEN 'e' THEN 'ENUM'
        WHEN 'c' THEN 'COMPOSITE'
        ELSE 'OTHER'
    END AS type_kind,
    COALESCE(d.description, '') AS type_comment
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = t.oid AND d.classoid = 'pg_type'::regclass
LEFT JOIN pg_class c ON t.typrelid = c.oid
WHERE t.typtype IN ('e', 'c')  -- ENUM and composite types only
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND (t.typtype = 'e' OR (t.typtype = 'c' AND c.relkind = 'c'))  -- For composite types, only include true composite types (not table types)
ORDER BY n.nspname, t.typname;

-- GetEnumValues retrieves enum values for ENUM types
-- name: GetEnumValues :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    e.enumlabel AS enum_value,
    e.enumsortorder AS enum_order
FROM pg_enum e
JOIN pg_type t ON e.enumtypid = t.oid
JOIN pg_namespace n ON t.typnamespace = n.oid
WHERE n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, t.typname, e.enumsortorder;

-- GetCompositeTypeColumns retrieves columns for composite types
-- name: GetCompositeTypeColumns :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    a.attname AS column_name,
    a.attnum AS column_position,
    format_type(a.atttypid, a.atttypmod) AS column_type
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
JOIN pg_class c ON t.typrelid = c.oid
JOIN pg_attribute a ON c.oid = a.attrelid
WHERE t.typtype = 'c'  -- composite types only
    AND c.relkind = 'c'  -- only true composite types, not table types
    AND a.attnum > 0  -- exclude system columns
    AND NOT a.attisdropped  -- exclude dropped columns
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, t.typname, a.attnum;

-- GetTriggers retrieves all triggers
-- name: GetTriggers :many
SELECT 
    trigger_schema,
    trigger_name,
    event_object_table,
    action_timing,
    event_manipulation,
    action_statement,
    action_condition,
    action_orientation
FROM information_schema.triggers
WHERE 
    trigger_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND trigger_schema NOT LIKE 'pg_temp_%'
    AND trigger_schema NOT LIKE 'pg_toast_temp_%'
ORDER BY trigger_schema, event_object_table, trigger_name;

-- GetViewDependencies retrieves view dependencies on tables and other views  
-- name: GetViewDependencies :many
SELECT DISTINCT
    vtu.view_schema AS dependent_schema,
    vtu.view_name AS dependent_name,
    vtu.table_schema AS source_schema,
    vtu.table_name AS source_name
FROM information_schema.view_table_usage vtu
WHERE 
    vtu.view_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND vtu.view_schema NOT LIKE 'pg_temp_%'
    AND vtu.view_schema NOT LIKE 'pg_toast_temp_%'
    AND vtu.table_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND vtu.table_schema NOT LIKE 'pg_temp_%'
    AND vtu.table_schema NOT LIKE 'pg_toast_temp_%'
ORDER BY vtu.view_schema, vtu.view_name, vtu.table_schema, vtu.table_name;

-- GetRLSTables retrieves tables with row level security enabled
-- name: GetRLSTables :many
SELECT
    n.nspname AS schemaname,
    c.relname AS tablename,
    c.relrowsecurity AS rowsecurity,
    c.relforcerowsecurity AS rowforced
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
WHERE
    n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND c.relkind = 'r'
    AND c.relrowsecurity = true
ORDER BY n.nspname, c.relname;

-- GetRLSPolicies retrieves all row level security policies
-- name: GetRLSPolicies :many
SELECT 
    schemaname,
    tablename,
    policyname,
    permissive,
    roles,
    cmd,
    qual,
    with_check
FROM pg_policies
WHERE 
    schemaname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND schemaname NOT LIKE 'pg_temp_%'
    AND schemaname NOT LIKE 'pg_toast_temp_%'
ORDER BY schemaname, tablename, policyname;

-- GetRLSTablesForSchema retrieves tables with row level security enabled for a specific schema
-- name: GetRLSTablesForSchema :many
SELECT
    n.nspname AS schemaname,
    c.relname AS tablename,
    c.relrowsecurity AS rowsecurity,
    c.relforcerowsecurity AS rowforced
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
WHERE
    n.nspname = $1
    AND c.relkind = 'r'
    AND c.relrowsecurity = true
ORDER BY n.nspname, c.relname;

-- GetRLSPoliciesForSchema retrieves all row level security policies for a specific schema
-- name: GetRLSPoliciesForSchema :many
SELECT 
    schemaname,
    tablename,
    policyname,
    permissive,
    roles,
    cmd,
    qual,
    with_check
FROM pg_policies
WHERE 
    schemaname = $1
ORDER BY schemaname, tablename, policyname;

-- GetDomains retrieves all user-defined domains
-- name: GetDomains :many
SELECT 
    n.nspname AS domain_schema,
    t.typname AS domain_name,
    format_type(t.typbasetype, t.typtypmod) AS base_type,
    t.typnotnull AS not_null,
    t.typdefault AS default_value,
    COALESCE(d.description, '') AS domain_comment
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = t.oid AND d.classoid = 'pg_type'::regclass
WHERE t.typtype = 'd'  -- Domain types only
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, t.typname;

-- GetDomainConstraints retrieves constraints for domains
-- name: GetDomainConstraints :many
SELECT 
    n.nspname AS domain_schema,
    t.typname AS domain_name,
    c.conname AS constraint_name,
    pg_get_constraintdef(c.oid, true) AS constraint_definition
FROM pg_constraint c
JOIN pg_type t ON c.contypid = t.oid
JOIN pg_namespace n ON t.typnamespace = n.oid
WHERE t.typtype = 'd'  -- Domain types only
    AND n.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
ORDER BY n.nspname, t.typname, c.conname;

-- GetPartitionedTablesForSchema retrieves partition information for partitioned tables in a specific schema
-- name: GetPartitionedTablesForSchema :many
SELECT 
    n.nspname AS table_schema,
    c.relname AS table_name,
    CASE pt.partstrat
        WHEN 'r' THEN 'RANGE'
        WHEN 'l' THEN 'LIST'
        WHEN 'h' THEN 'HASH'
        ELSE 'UNKNOWN'
    END AS partition_strategy,
    STRING_AGG(a.attname, ', ' ORDER BY a.attnum) AS partition_key
FROM pg_partitioned_table pt
JOIN pg_class c ON pt.partrelid = c.oid
JOIN pg_namespace n ON c.relnamespace = n.oid
JOIN pg_attribute a ON a.attrelid = pt.partrelid AND a.attnum = ANY(pt.partattrs)
WHERE n.nspname = $1
GROUP BY n.nspname, c.relname, pt.partstrat
ORDER BY n.nspname, c.relname;

-- GetPartitionChildren retrieves partition child tables and their attachment information
-- name: GetPartitionChildren :many
SELECT 
    pn.nspname AS parent_schema,
    pc.relname AS parent_table,
    cn.nspname AS child_schema,
    cc.relname AS child_table,
    pg_get_expr(cc.relpartbound, cc.oid) AS partition_bound
FROM pg_inherits inh
JOIN pg_class pc ON inh.inhparent = pc.oid
JOIN pg_namespace pn ON pc.relnamespace = pn.oid
JOIN pg_class cc ON inh.inhrelid = cc.oid
JOIN pg_namespace cn ON cc.relnamespace = cn.oid
WHERE pn.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND pn.nspname NOT LIKE 'pg_temp_%'
    AND pn.nspname NOT LIKE 'pg_toast_temp_%'
    AND cn.nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
    AND cn.nspname NOT LIKE 'pg_temp_%'
    AND cn.nspname NOT LIKE 'pg_toast_temp_%'
    AND EXISTS (
        SELECT 1 FROM pg_partitioned_table pt 
        WHERE pt.partrelid = pc.oid
    )
ORDER BY pn.nspname, pc.relname, cn.nspname, cc.relname;


-- GetConstraintsForSchema retrieves all table constraints for a specific schema
-- name: GetConstraintsForSchema :many
SELECT 
    n.nspname AS table_schema,
    cl.relname AS table_name,
    c.conname AS constraint_name,
    CASE c.contype
        WHEN 'c' THEN 'CHECK'
        WHEN 'f' THEN 'FOREIGN KEY'
        WHEN 'p' THEN 'PRIMARY KEY'
        WHEN 'u' THEN 'UNIQUE'
        WHEN 'x' THEN 'EXCLUDE'
        ELSE 'UNKNOWN'
    END AS constraint_type,
    COALESCE(a.attname, '') AS column_name,
    COALESCE(a.attnum, 0) AS ordinal_position,
    COALESCE(fn.nspname, '') AS foreign_table_schema,
    COALESCE(fcl.relname, '') AS foreign_table_name,
    COALESCE(fa.attname, '') AS foreign_column_name,
    COALESCE(fa.attnum, 0) AS foreign_ordinal_position,
    CASE WHEN c.contype = 'c' THEN pg_get_constraintdef(c.oid, true) ELSE NULL END AS check_clause,
    CASE WHEN c.contype = 'x' THEN pg_get_constraintdef(c.oid, true) ELSE NULL END AS exclusion_definition,
    CASE c.confdeltype
        WHEN 'a' THEN 'NO ACTION'
        WHEN 'r' THEN 'RESTRICT'
        WHEN 'c' THEN 'CASCADE'
        WHEN 'n' THEN 'SET NULL'
        WHEN 'd' THEN 'SET DEFAULT'
        ELSE NULL
    END AS delete_rule,
    CASE c.confupdtype
        WHEN 'a' THEN 'NO ACTION'
        WHEN 'r' THEN 'RESTRICT'
        WHEN 'c' THEN 'CASCADE'
        WHEN 'n' THEN 'SET NULL'
        WHEN 'd' THEN 'SET DEFAULT'
        ELSE NULL
    END AS update_rule,
    c.condeferrable AS deferrable,
    c.condeferred AS initially_deferred,
    c.convalidated AS is_valid,
    COALESCE((to_jsonb(c) ->> 'conperiod')::boolean, false) AS is_period,
    c.connoinherit AS no_inherit
FROM pg_constraint c
JOIN pg_class cl ON c.conrelid = cl.oid
JOIN pg_namespace n ON cl.relnamespace = n.oid
LEFT JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
LEFT JOIN pg_class fcl ON c.confrelid = fcl.oid
LEFT JOIN pg_namespace fn ON fcl.relnamespace = fn.oid
LEFT JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = c.confkey[array_position(c.conkey, a.attnum)]
WHERE n.nspname = $1
ORDER BY n.nspname, cl.relname, c.contype, c.conname, a.attnum;

-- GetSequencesForSchema retrieves all sequences for a specific schema
-- name: GetSequencesForSchema :many
SELECT 
    s.schemaname AS sequence_schema,
    s.sequencename AS sequence_name,
    s.data_type,
    s.start_value,
    s.min_value AS minimum_value,
    s.max_value AS maximum_value,
    s.increment_by AS increment,
    s.cycle AS cycle_option,
    s.cache_size,
    COALESCE(dep_table.relname, col_table.table_name) AS owned_by_table,
    COALESCE(dep_col.attname, col_table.column_name) AS owned_by_column
FROM pg_sequences s
LEFT JOIN pg_namespace n ON n.nspname = s.schemaname
LEFT JOIN pg_class c ON c.relname = s.sequencename AND c.relnamespace = n.oid
-- Method 1: Try to find dependency relationship (for proper SERIAL columns)
LEFT JOIN pg_depend d ON d.objid = c.oid AND d.classid = 'pg_class'::regclass AND d.deptype IN ('a', 'i')
LEFT JOIN pg_class dep_table ON d.refobjid = dep_table.oid
LEFT JOIN pg_attribute dep_col ON dep_col.attrelid = dep_table.oid AND dep_col.attnum = d.refobjsubid
-- Method 2: Find sequences used in column defaults (for nextval() patterns)
LEFT JOIN (
    SELECT 
        col.table_name,
        col.column_name,
        REGEXP_REPLACE(
            REGEXP_REPLACE(col.column_default, 'nextval\(''([^'']+)''.*\)', '\1'),
            '^[^.]*\.', ''
        ) AS sequence_name
    FROM information_schema.columns col
    WHERE col.table_schema = $1
      AND col.column_default LIKE '%nextval%'
) col_table ON col_table.sequence_name = s.sequencename
WHERE s.schemaname = $1
ORDER BY s.schemaname, s.sequencename;

-- GetFunctionsForSchema retrieves all user-defined functions for a specific schema
-- name: GetFunctionsForSchema :many
SELECT
    r.routine_schema,
    r.routine_name,
    -- Use pg_get_function_sqlbody for RETURN clause syntax (PG14+)
    -- Fall back to prosrc for traditional AS $$ ... $$ syntax
    COALESCE(
        pg_get_function_sqlbody(p.oid),
        CASE WHEN p.prosrc ~ E'\n$' THEN p.prosrc ELSE p.prosrc || E'\n' END
    ) AS routine_definition,
    r.routine_type,
    COALESCE(pg_get_function_result(p.oid), r.data_type) AS data_type,
    r.external_language,
    COALESCE(desc_func.description, '') AS function_comment,
    oidvectortypes(p.proargtypes) AS function_arguments,
    pg_get_function_arguments(p.oid) AS function_signature,
    CASE p.provolatile
        WHEN 'i' THEN 'IMMUTABLE'
        WHEN 's' THEN 'STABLE'
        WHEN 'v' THEN 'VOLATILE'
        ELSE NULL
    END AS volatility,
    p.proisstrict AS is_strict,
    p.prosecdef AS is_security_definer,
    p.proleakproof AS is_leakproof,
    p.proparallel AS parallel_mode,
    (SELECT substring(cfg FROM 'search_path=(.*)') FROM unnest(p.proconfig) AS cfg WHERE cfg LIKE 'search_path=%') AS search_path
FROM information_schema.routines r
LEFT JOIN pg_proc p ON p.proname = r.routine_name
    AND p.pronamespace = (SELECT oid FROM pg_namespace WHERE nspname = r.routine_schema)
    AND p.oid = (regexp_match(r.specific_name, '_(\d+)$'))[1]::oid
LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
LEFT JOIN pg_description desc_func ON desc_func.objoid = p.oid AND desc_func.classoid = 'pg_proc'::regclass
WHERE r.routine_schema = $1
    AND r.routine_type = 'FUNCTION'
    AND d.objid IS NULL  -- Exclude functions that are extension members
ORDER BY r.routine_schema, r.routine_name;

-- GetProceduresForSchema retrieves all user-defined procedures for a specific schema
-- name: GetProceduresForSchema :many
SELECT
    r.routine_schema,
    r.routine_name,
    -- Use pg_get_function_sqlbody for RETURN clause syntax (PG14+)
    -- Fall back to prosrc for traditional AS $$ ... $$ syntax
    COALESCE(
        pg_get_function_sqlbody(p.oid),
        CASE WHEN p.prosrc ~ E'\n$' THEN p.prosrc ELSE p.prosrc || E'\n' END
    ) AS routine_definition,
    r.routine_type,
    r.external_language,
    COALESCE(desc_proc.description, '') AS procedure_comment,
    oidvectortypes(p.proargtypes) AS procedure_arguments,
    pg_get_function_arguments(p.oid) AS procedure_signature
FROM information_schema.routines r
LEFT JOIN pg_proc p ON p.proname = r.routine_name
    AND p.pronamespace = (SELECT oid FROM pg_namespace WHERE nspname = r.routine_schema)
    AND p.oid = (regexp_match(r.specific_name, '_(\d+)$'))[1]::oid
LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
LEFT JOIN pg_description desc_proc ON desc_proc.objoid = p.oid AND desc_proc.classoid = 'pg_proc'::regclass
WHERE r.routine_schema = $1
    AND r.routine_type = 'PROCEDURE'
    AND d.objid IS NULL  -- Exclude procedures that are extension members
ORDER BY r.routine_schema, r.routine_name;

-- GetAggregatesForSchema retrieves all user-defined aggregates for a specific schema
-- name: GetAggregatesForSchema :many
SELECT 
    n.nspname AS aggregate_schema,
    p.proname AS aggregate_name,
    pg_get_function_arguments(p.oid) AS aggregate_signature,
    oidvectortypes(p.proargtypes) AS aggregate_arguments,
    format_type(p.prorettype, NULL) AS aggregate_return_type,
    -- Get transition function
    COALESCE(tf.proname, '') AS transition_function,
    COALESCE(tfn.nspname, '') AS transition_function_schema,
    -- Get state type
    format_type(a.aggtranstype, NULL) AS state_type,
    -- Get initial condition
    a.agginitval AS initial_condition,
    -- Get final function if exists
    COALESCE(ff.proname, '') AS final_function,
    COALESCE(ffn.nspname, '') AS final_function_schema,
    -- Comment
    COALESCE(d.description, '') AS aggregate_comment
FROM pg_proc p
JOIN pg_namespace n ON p.pronamespace = n.oid
JOIN pg_aggregate a ON a.aggfnoid = p.oid
LEFT JOIN pg_proc tf ON a.aggtransfn = tf.oid
LEFT JOIN pg_namespace tfn ON tf.pronamespace = tfn.oid
LEFT JOIN pg_proc ff ON a.aggfinalfn = ff.oid
LEFT JOIN pg_namespace ffn ON ff.pronamespace = ffn.oid
LEFT JOIN pg_description d ON d.objoid = p.oid AND d.classoid = 'pg_proc'::regclass
WHERE p.prokind = 'a'  -- Only aggregates
    AND n.nspname = $1
    AND NOT EXISTS (
        SELECT 1 FROM pg_depend dep 
        WHERE dep.objid = p.oid AND dep.deptype = 'e'
    )  -- Exclude extension members
ORDER BY n.nspname, p.proname;

-- GetViewsForSchema retrieves all views and materialized views for a specific schema
-- IMPORTANT: Uses LATERAL join with set_config to temporarily set search_path to only the view's schema
-- This ensures pg_get_viewdef() includes schema qualifiers for cross-schema references
-- The LATERAL join guarantees set_config executes before pg_get_viewdef in the same row context
-- name: GetViewsForSchema :many
WITH view_definitions AS (
    SELECT
        n.nspname AS table_schema,
        c.relname AS table_name,
        c.oid AS view_oid,
        COALESCE(d.description, '') AS view_comment,
        (c.relkind = 'm') AS is_materialized,
        n.nspname AS view_schema,
        c.reloptions AS reloptions
    FROM pg_class c
    JOIN pg_namespace n ON c.relnamespace = n.oid
    LEFT JOIN pg_description d ON d.objoid = c.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = 0
    WHERE
        c.relkind IN ('v', 'm') -- views and materialized views
        AND n.nspname = $1
)
SELECT
    vd.table_schema,
    vd.table_name,
    -- Use LATERAL join to guarantee execution order:
    -- 1. set_config sets search_path to only the view's schema
    -- 2. pg_get_viewdef then uses that search_path
    -- This ensures cross-schema table references are qualified with schema names
    sp.view_def AS view_definition,
    vd.view_comment,
    vd.is_materialized,
    vd.reloptions
FROM view_definitions vd
CROSS JOIN LATERAL (
    SELECT
        set_config('search_path', vd.view_schema || ', pg_catalog', true) as dummy,
        pg_get_viewdef(vd.view_oid, true) as view_def
) sp
ORDER BY vd.table_schema, vd.table_name;

-- GetTriggersForSchema retrieves all triggers for a specific schema
-- Uses pg_trigger catalog to include all trigger types (including TRUNCATE)
-- which are not visible in information_schema.triggers
-- name: GetTriggersForSchema :many
SELECT
    n.nspname AS trigger_schema,
    c.relname AS event_object_table,
    t.tgname AS trigger_name,
    t.tgtype AS trigger_type,
    t.tgenabled AS trigger_enabled,
    t.tgdeferrable AS trigger_deferrable,
    t.tginitdeferred AS trigger_initdeferred,
    t.tgconstraint AS trigger_constraint_oid,
    COALESCE(pg_catalog.pg_get_triggerdef(t.oid), '') AS trigger_definition,
    COALESCE(t.tgoldtable, '') AS old_table,
    COALESCE(t.tgnewtable, '') AS new_table,
    p.proname AS function_name,
    pn.nspname AS function_schema,
    COALESCE(d.description, '') AS trigger_comment
FROM pg_catalog.pg_trigger t
JOIN pg_catalog.pg_class c ON t.tgrelid = c.oid
JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
JOIN pg_catalog.pg_proc p ON t.tgfoid = p.oid
JOIN pg_catalog.pg_namespace pn ON p.pronamespace = pn.oid
LEFT JOIN pg_description d ON d.objoid = t.oid AND d.classoid = 'pg_trigger'::regclass
WHERE n.nspname = $1
    AND NOT t.tgisinternal  -- Exclude internal triggers
ORDER BY n.nspname, c.relname, t.tgname;

-- GetTypesForSchema retrieves all user-defined types for a specific schema
-- name: GetTypesForSchema :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    CASE t.typtype
        WHEN 'e' THEN 'ENUM'
        WHEN 'c' THEN 'COMPOSITE'
        ELSE 'OTHER'
    END AS type_kind,
    COALESCE(d.description, '') AS type_comment
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = t.oid AND d.classoid = 'pg_type'::regclass
LEFT JOIN pg_class c ON t.typrelid = c.oid
WHERE t.typtype IN ('e', 'c')  -- ENUM and composite types only
    AND n.nspname = $1
    AND (t.typtype = 'e' OR (t.typtype = 'c' AND c.relkind = 'c'))  -- For composite types, only include true composite types (not table types)
ORDER BY n.nspname, t.typname;

-- GetDomainsForSchema retrieves all user-defined domains for a specific schema
-- name: GetDomainsForSchema :many
SELECT 
    n.nspname AS domain_schema,
    t.typname AS domain_name,
    format_type(t.typbasetype, t.typtypmod) AS base_type,
    t.typnotnull AS not_null,
    t.typdefault AS default_value,
    COALESCE(d.description, '') AS domain_comment
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
LEFT JOIN pg_description d ON d.objoid = t.oid AND d.classoid = 'pg_type'::regclass
WHERE t.typtype = 'd'  -- Domain types only
    AND n.nspname = $1
ORDER BY n.nspname, t.typname;

-- GetDomainConstraintsForSchema retrieves constraints for domains in a specific schema
-- name: GetDomainConstraintsForSchema :many
SELECT 
    n.nspname AS domain_schema,
    t.typname AS domain_name,
    c.conname AS constraint_name,
    pg_get_constraintdef(c.oid, true) AS constraint_definition
FROM pg_constraint c
JOIN pg_type t ON c.contypid = t.oid
JOIN pg_namespace n ON t.typnamespace = n.oid
WHERE t.typtype = 'd'  -- Domain types only
    AND n.nspname = $1
ORDER BY n.nspname, t.typname, c.conname;

-- GetEnumValuesForSchema retrieves enum values for ENUM types in a specific schema
-- name: GetEnumValuesForSchema :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    e.enumlabel AS enum_value,
    e.enumsortorder AS enum_order
FROM pg_enum e
JOIN pg_type t ON e.enumtypid = t.oid
JOIN pg_namespace n ON t.typnamespace = n.oid
WHERE n.nspname = $1
ORDER BY n.nspname, t.typname, e.enumsortorder;

-- GetCompositeTypeColumnsForSchema retrieves columns for composite types in a specific schema
-- name: GetCompositeTypeColumnsForSchema :many
SELECT 
    n.nspname AS type_schema,
    t.typname AS type_name,
    a.attname AS column_name,
    a.attnum AS column_position,
    format_type(a.atttypid, a.atttypmod) AS column_type
FROM pg_type t
JOIN pg_namespace n ON t.typnamespace = n.oid
JOIN pg_class c ON t.typrelid = c.oid
JOIN pg_attribute a ON c.oid = a.attrelid
WHERE t.typtype = 'c'  -- composite types only
    AND c.relkind = 'c'  -- only true composite types, not table types
    AND a.attnum > 0  -- exclude system columns
    AND NOT a.attisdropped  -- exclude dropped columns
    AND n.nspname = $1
ORDER BY n.nspname, t.typname, a.attnum;

-- GetDefaultPrivilegesForSchema retrieves default privileges for a specific schema
-- name: GetDefaultPrivilegesForSchema :many
WITH acl_expanded AS (
    SELECT
        d.defaclrole,
        d.defaclobjtype,
        (aclexplode(d.defaclacl)).grantee AS grantee_oid,
        (aclexplode(d.defaclacl)).privilege_type AS privilege_type,
        (aclexplode(d.defaclacl)).is_grantable AS is_grantable
    FROM pg_default_acl d
    JOIN pg_namespace n ON d.defaclnamespace = n.oid
    WHERE n.nspname = $1
        -- Only include privileges for roles the current user can manage.
        -- pg_has_role returns true for superusers (who can manage any role)
        -- and for roles the current user is a member of. This prevents
        -- generating diffs for system roles (e.g., supabase_admin) that
        -- the current user cannot actually modify via ALTER DEFAULT PRIVILEGES.
        AND pg_has_role(current_user, d.defaclrole, 'MEMBER')
)
SELECT
    pg_get_userbyid(a.defaclrole) AS owner_role,
    CASE a.defaclobjtype
        WHEN 'r' THEN 'TABLES'
        WHEN 'S' THEN 'SEQUENCES'
        WHEN 'f' THEN 'FUNCTIONS'
        WHEN 'T' THEN 'TYPES'
        WHEN 'n' THEN 'SCHEMAS'
    END AS object_type,
    COALESCE(r.rolname, 'PUBLIC') AS grantee,
    a.privilege_type,
    a.is_grantable
FROM acl_expanded a
LEFT JOIN pg_roles r ON a.grantee_oid = r.oid
ORDER BY owner_role, object_type, grantee, privilege_type;

-- GetPrivilegesForSchema retrieves explicit privilege grants for objects in a specific schema
-- name: GetPrivilegesForSchema :many
WITH acl_data AS (
    -- Tables and Views
    SELECT
        n.nspname AS schema_name,
        c.relname::text AS object_name,
        CASE c.relkind
            WHEN 'r' THEN 'TABLE'
            WHEN 'v' THEN 'VIEW'
            WHEN 'm' THEN 'VIEW'
            WHEN 'S' THEN 'SEQUENCE'
        END AS object_type,
        c.relacl AS acl,
        pg_get_userbyid(c.relowner) AS owner
    FROM pg_class c
    JOIN pg_namespace n ON c.relnamespace = n.oid
    WHERE n.nspname = $1
        AND c.relkind IN ('r', 'v', 'm', 'S')
        AND c.relacl IS NOT NULL

    UNION ALL

    -- Functions
    SELECT
        n.nspname AS schema_name,
        p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')' AS object_name,
        'FUNCTION' AS object_type,
        p.proacl AS acl,
        pg_get_userbyid(p.proowner) AS owner
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = $1
        AND p.prokind = 'f'
        AND p.proacl IS NOT NULL

    UNION ALL

    -- Procedures
    SELECT
        n.nspname AS schema_name,
        p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')' AS object_name,
        'PROCEDURE' AS object_type,
        p.proacl AS acl,
        pg_get_userbyid(p.proowner) AS owner
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = $1
        AND p.prokind = 'p'
        AND p.proacl IS NOT NULL

    UNION ALL

    -- Types (ENUM, COMPOSITE, DOMAIN)
    SELECT
        n.nspname AS schema_name,
        t.typname::text AS object_name,
        'TYPE' AS object_type,
        t.typacl AS acl,
        pg_get_userbyid(t.typowner) AS owner
    FROM pg_type t
    JOIN pg_namespace n ON t.typnamespace = n.oid
    WHERE n.nspname = $1
        AND t.typtype IN ('e', 'c', 'd')
        AND t.typacl IS NOT NULL
)
SELECT
    schema_name,
    object_name,
    object_type,
    (aclexplode(acl)).grantee AS grantee_oid,
    (aclexplode(acl)).privilege_type AS privilege_type,
    (aclexplode(acl)).is_grantable AS is_grantable,
    owner
FROM acl_data
ORDER BY object_type, object_name, grantee_oid, privilege_type;

-- GetRevokedDefaultPrivilegesForSchema finds objects where default PUBLIC grants have been explicitly revoked
-- name: GetRevokedDefaultPrivilegesForSchema :many
WITH objects_with_acl AS (
    -- Functions (ACL may be NULL for default permissions; filtering happens in public_grants CTE)
    SELECT
        p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')' AS object_name,
        'FUNCTION' AS object_type,
        p.proacl AS acl
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = $1
        AND p.prokind = 'f'

    UNION ALL

    -- Procedures
    SELECT
        p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')' AS object_name,
        'PROCEDURE' AS object_type,
        p.proacl AS acl
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = $1
        AND p.prokind = 'p'

    UNION ALL

    -- Types
    SELECT
        t.typname::text AS object_name,
        'TYPE' AS object_type,
        t.typacl AS acl
    FROM pg_type t
    JOIN pg_namespace n ON t.typnamespace = n.oid
    WHERE n.nspname = $1
        AND t.typtype IN ('e', 'c', 'd')
),
public_grants AS (
    SELECT
        object_name,
        object_type,
        EXISTS (
            SELECT 1
            FROM unnest(acl) AS acl_entry
            WHERE acl_entry::text LIKE '=%'  -- PUBLIC grants start with =
        ) AS has_public_grant,
        acl IS NOT NULL AS has_explicit_acl
    FROM objects_with_acl
)
SELECT object_name, object_type
FROM public_grants
WHERE has_explicit_acl = true AND has_public_grant = false
ORDER BY object_type, object_name;

-- GetColumnPrivilegesForSchema retrieves column-level privilege grants
-- Column privileges are stored in pg_attribute.attacl and allow fine-grained access
-- name: GetColumnPrivilegesForSchema :many
WITH column_acls AS (
    SELECT
        c.relname AS table_name,
        a.attname AS column_name,
        a.attacl AS acl
    FROM pg_attribute a
    JOIN pg_class c ON a.attrelid = c.oid
    JOIN pg_namespace n ON c.relnamespace = n.oid
    WHERE n.nspname = $1
        AND c.relkind IN ('r', 'v', 'm')  -- tables, views, materialized views
        AND a.attnum > 0                   -- skip system columns
        AND NOT a.attisdropped
        AND a.attacl IS NOT NULL           -- only columns with explicit ACL
)
SELECT
    table_name,
    column_name,
    (aclexplode(acl)).grantee AS grantee_oid,
    (aclexplode(acl)).privilege_type AS privilege_type,
    (aclexplode(acl)).is_grantable AS is_grantable
FROM column_acls
ORDER BY table_name, column_name, grantee_oid, privilege_type;

-- GetFunctionDependencies retrieves function-to-function dependencies for topological sorting
-- name: GetFunctionDependencies :many
SELECT
    dependent_ns.nspname AS dependent_schema,
    dependent_proc.proname AS dependent_name,
    pg_get_function_identity_arguments(dependent_proc.oid) AS dependent_args,
    referenced_ns.nspname AS referenced_schema,
    referenced_proc.proname AS referenced_name,
    pg_get_function_identity_arguments(referenced_proc.oid) AS referenced_args
FROM pg_depend d
JOIN pg_proc dependent_proc ON d.objid = dependent_proc.oid
JOIN pg_namespace dependent_ns ON dependent_proc.pronamespace = dependent_ns.oid
JOIN pg_proc referenced_proc ON d.refobjid = referenced_proc.oid
JOIN pg_namespace referenced_ns ON referenced_proc.pronamespace = referenced_ns.oid
WHERE d.classid = 'pg_proc'::regclass
  AND d.refclassid = 'pg_proc'::regclass
  AND d.deptype = 'n'
  AND dependent_ns.nspname = $1;