CREATE TABLE IF NOT EXISTS activity (
    id uuid,
    author_id uuid,
    CONSTRAINT activity_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS contact (
    id uuid,
    name text NOT NULL,
    CONSTRAINT contact_pkey PRIMARY KEY (id)
);

CREATE OR REPLACE VIEW actor AS
 SELECT id,
    name
   FROM contact;

CREATE OR REPLACE FUNCTION get_actor(
    activity activity
)
RETURNS SETOF actor
LANGUAGE sql
STABLE
ROWS 1
AS $$ SELECT actor.* FROM actor WHERE actor.id = activity.author_id
$$;
