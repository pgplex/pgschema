CREATE TABLE IF NOT EXISTS organizations (
    tenant_id integer,
    org_id integer,
    org_name text NOT NULL,
    org_type text NOT NULL,
    CONSTRAINT organizations_pkey PRIMARY KEY (tenant_id, org_id),
    CONSTRAINT organizations_org_type_org_id_tenant_id_key UNIQUE (org_type, org_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS projects (
    tenant_id integer,
    org_id integer,
    project_id integer,
    project_name text NOT NULL,
    project_code text NOT NULL,
    description text,
    CONSTRAINT projects_pkey PRIMARY KEY (tenant_id, org_id, project_id),
    CONSTRAINT projects_project_name_tenant_id_project_id_key UNIQUE (project_name, tenant_id, project_id),
    CONSTRAINT projects_tenant_id_org_id_fkey FOREIGN KEY (tenant_id, org_id) REFERENCES organizations (tenant_id, org_id) MATCH FULL
);
