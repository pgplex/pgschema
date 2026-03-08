CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    email VARCHAR(100),
    bio TEXT,
    status VARCHAR(20) NOT NULL,
    department_id INTEGER,
    priority INTEGER
);

CREATE TABLE public.departments (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    manager_id INTEGER
);

-- View testing array operators: all ANY/ALL operators are preserved
CREATE VIEW public.array_operators_view AS
SELECT
    id,
    priority,
    -- All ANY operations preserve the ANY syntax
    CASE WHEN priority = ANY(ARRAY[10, 20, 30]) THEN 'matched' ELSE 'not_matched' END AS equal_any_test,
    CASE WHEN priority > ANY(ARRAY[10, 20, 30]) THEN 'high' ELSE 'low' END AS greater_any_test,
    CASE WHEN priority < ANY(ARRAY[5, 15, 25]) THEN 'found_lower' ELSE 'all_higher' END AS less_any_test,
    CASE WHEN priority <> ANY(ARRAY[1, 2, 3]) THEN 'different' ELSE 'same' END AS not_equal_any_test
FROM employees;

-- View testing COALESCE, string concatenation, and to_tsvector for full text search
CREATE VIEW public.text_search_view AS
SELECT
    id,
    COALESCE(first_name || ' ' || last_name, 'Anonymous') AS display_name,
    COALESCE(email, '') AS email,
    COALESCE(bio, 'No description available') AS description,
    to_tsvector('english', COALESCE(first_name, '') || ' ' || COALESCE(last_name, '') || ' ' || COALESCE(bio, '')) AS search_vector
FROM employees
WHERE status = 'active';

-- View testing NULLIF, GREATEST, LEAST, and joins (regression test for issue #103)
CREATE VIEW public.nullif_functions_view AS
SELECT
    e.id,
    e.name AS employee_name,
    d.name AS department_name,
    -- NULLIF to avoid divide-by-zero (main issue from #103)
    (e.priority - d.manager_id) / NULLIF(d.manager_id, 0) AS priority_ratio,
    -- Multiple NULLIF expressions
    NULLIF(e.status, 'inactive') AS active_status,
    NULLIF(e.email, '') AS valid_email,
    -- GREATEST and LEAST functions
    GREATEST(e.priority, 0) AS min_priority,
    LEAST(e.priority, 100) AS max_priority,
    GREATEST(e.id, d.id, e.department_id) AS max_id,
    -- Complex CASE with NULLIF
    CASE
        WHEN NULLIF(e.department_id, 0) IS NOT NULL THEN 'assigned'
        ELSE 'unassigned'
    END AS assignment_status
FROM employees e
JOIN departments d USING (id)
WHERE e.priority > 0;

-- View testing CTEs with CASE statements (regression test for issue #106)
CREATE VIEW public.cte_with_case_view AS
WITH monthly_stats AS (
    SELECT
        date_trunc('month', CURRENT_DATE - (n || ' months')::INTERVAL) AS month_start,
        n AS month_offset
    FROM generate_series(0, 11) AS n
),
employee_summary AS (
    SELECT
        department_id,
        COUNT(*) AS employee_count,
        AVG(priority) AS avg_priority
    FROM employees
    WHERE status = 'active'
    GROUP BY department_id
)
SELECT
    ms.month_start,
    ms.month_offset,
    d.name AS department_name,
    COALESCE(es.employee_count, 0) AS employee_count,
    -- CASE statement using CTE data (triggers the bug from #106)
    CASE
        WHEN es.avg_priority > 50 THEN 'high'
        WHEN es.avg_priority > 25 THEN 'medium'
        WHEN es.avg_priority IS NOT NULL THEN 'low'
        ELSE 'no_data'
    END AS priority_level,
    -- Another CASE with CTE
    CASE
        WHEN ms.month_offset = 0 THEN 'current'
        WHEN ms.month_offset <= 3 THEN 'recent'
        ELSE 'historical'
    END AS period_type
FROM monthly_stats ms
CROSS JOIN departments d
LEFT JOIN employee_summary es ON d.id = es.department_id
ORDER BY ms.month_start DESC, d.name;

-- View testing UNION and UNION ALL in subqueries (regression test for issue #104)
CREATE VIEW public.union_subquery_view AS
SELECT
    id,
    name,
    source_type,
    -- Additional columns from the union result
    CASE
        WHEN source_type = 'employee' THEN 'active'
        WHEN source_type = 'department' THEN 'organizational'
        ELSE 'unknown'
    END AS category
FROM (
    -- Simple UNION combining employees and departments (main issue from #104)
    SELECT
        id,
        name,
        'employee' AS source_type
    FROM employees
    WHERE status = 'active'
    UNION
    SELECT
        id,
        name,
        'department' AS source_type
    FROM departments
    WHERE manager_id IS NOT NULL
    -- UNION ALL variant (keep duplicates)
    UNION ALL
    SELECT
        id,
        COALESCE(first_name || ' ' || last_name, name) AS name,
        'employee_full' AS source_type
    FROM employees
    WHERE priority > 10
) AS combined_data
WHERE id IS NOT NULL
ORDER BY source_type, id;

-- View with security_invoker option (PG 15+, issue #343)
CREATE VIEW public.secure_employee_view WITH (security_invoker = true) AS
SELECT
    id,
    name,
    email,
    status
FROM employees
WHERE status = 'active';
