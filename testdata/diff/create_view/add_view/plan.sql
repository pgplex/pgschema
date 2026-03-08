CREATE OR REPLACE VIEW array_operators_view AS
 SELECT id,
    priority,
        CASE
            WHEN priority = ANY (ARRAY[10, 20, 30]) THEN 'matched'::text
            ELSE 'not_matched'::text
        END AS equal_any_test,
        CASE
            WHEN priority > ANY (ARRAY[10, 20, 30]) THEN 'high'::text
            ELSE 'low'::text
        END AS greater_any_test,
        CASE
            WHEN priority < ANY (ARRAY[5, 15, 25]) THEN 'found_lower'::text
            ELSE 'all_higher'::text
        END AS less_any_test,
        CASE
            WHEN priority <> ANY (ARRAY[1, 2, 3]) THEN 'different'::text
            ELSE 'same'::text
        END AS not_equal_any_test
   FROM employees;

CREATE OR REPLACE VIEW cte_with_case_view AS
 WITH monthly_stats AS (
         SELECT date_trunc('month'::text, CURRENT_DATE - ((n.n || ' months'::text)::interval)) AS month_start,
            n.n AS month_offset
           FROM generate_series(0, 11) n(n)
        ), employee_summary AS (
         SELECT employees.department_id,
            count(*) AS employee_count,
            avg(employees.priority) AS avg_priority
           FROM employees
          WHERE employees.status::text = 'active'::text
          GROUP BY employees.department_id
        )
 SELECT ms.month_start,
    ms.month_offset,
    d.name AS department_name,
    COALESCE(es.employee_count, 0::bigint) AS employee_count,
        CASE
            WHEN es.avg_priority > 50::numeric THEN 'high'::text
            WHEN es.avg_priority > 25::numeric THEN 'medium'::text
            WHEN es.avg_priority IS NOT NULL THEN 'low'::text
            ELSE 'no_data'::text
        END AS priority_level,
        CASE
            WHEN ms.month_offset = 0 THEN 'current'::text
            WHEN ms.month_offset <= 3 THEN 'recent'::text
            ELSE 'historical'::text
        END AS period_type
   FROM monthly_stats ms
     CROSS JOIN departments d
     LEFT JOIN employee_summary es ON d.id = es.department_id
  ORDER BY ms.month_start DESC, d.name;

CREATE OR REPLACE VIEW nullif_functions_view AS
 SELECT e.id,
    e.name AS employee_name,
    d.name AS department_name,
    (e.priority - d.manager_id) / NULLIF(d.manager_id, 0) AS priority_ratio,
    NULLIF(e.status::text, 'inactive'::text) AS active_status,
    NULLIF(e.email::text, ''::text) AS valid_email,
    GREATEST(e.priority, 0) AS min_priority,
    LEAST(e.priority, 100) AS max_priority,
    GREATEST(e.id, d.id, e.department_id) AS max_id,
        CASE
            WHEN NULLIF(e.department_id, 0) IS NOT NULL THEN 'assigned'::text
            ELSE 'unassigned'::text
        END AS assignment_status
   FROM employees e
     JOIN departments d USING (id)
  WHERE e.priority > 0;

CREATE OR REPLACE VIEW secure_employee_view WITH (security_invoker=true) AS
 SELECT id,
    name,
    email,
    status
   FROM employees
  WHERE status::text = 'active'::text;

CREATE OR REPLACE VIEW text_search_view AS
 SELECT id,
    COALESCE((first_name::text || ' '::text) || last_name::text, 'Anonymous'::text) AS display_name,
    COALESCE(email, ''::character varying) AS email,
    COALESCE(bio, 'No description available'::text) AS description,
    to_tsvector('english'::regconfig, (((COALESCE(first_name, ''::character varying)::text || ' '::text) || COALESCE(last_name, ''::character varying)::text) || ' '::text) || COALESCE(bio, ''::text)) AS search_vector
   FROM employees
  WHERE status::text = 'active'::text;

CREATE OR REPLACE VIEW union_subquery_view AS
 SELECT id,
    name,
    source_type,
        CASE
            WHEN source_type = 'employee'::text THEN 'active'::text
            WHEN source_type = 'department'::text THEN 'organizational'::text
            ELSE 'unknown'::text
        END AS category
   FROM ((
                 SELECT employees.id,
                    employees.name,
                    'employee'::text AS source_type
                   FROM employees
                  WHERE employees.status::text = 'active'::text
                UNION
                 SELECT departments.id,
                    departments.name,
                    'department'::text AS source_type
                   FROM departments
                  WHERE departments.manager_id IS NOT NULL
        ) UNION ALL
         SELECT employees.id,
            COALESCE((employees.first_name::text || ' '::text) || employees.last_name::text, employees.name::text) AS name,
            'employee_full'::text AS source_type
           FROM employees
          WHERE employees.priority > 10) combined_data
  WHERE id IS NOT NULL
  ORDER BY source_type, id;
