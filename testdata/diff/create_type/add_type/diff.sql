ALTER TYPE status ADD VALUE 'archived' AFTER 'pending';

CREATE TABLE IF NOT EXISTS work_archive (
    id integer,
    state status DEFAULT 'archived'::status NOT NULL,
    CONSTRAINT work_archive_pkey PRIMARY KEY (id)
);

ALTER TABLE work ALTER COLUMN state SET DEFAULT 'archived'::status;
