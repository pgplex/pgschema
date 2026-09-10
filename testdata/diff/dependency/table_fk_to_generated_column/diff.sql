CREATE TABLE IF NOT EXISTS articlesource (
    id integer,
    article_id integer NOT NULL,
    source_url text,
    CONSTRAINT articlesource_pkey PRIMARY KEY (id)
);

CREATE OR REPLACE FUNCTION calc_priority()
RETURNS integer
LANGUAGE sql
IMMUTABLE
AS $$SELECT 1
$$;

CREATE TABLE IF NOT EXISTS article (
    id integer,
    title text NOT NULL,
    priority integer GENERATED ALWAYS AS (calc_priority()) STORED,
    CONSTRAINT article_pkey PRIMARY KEY (id)
);

ALTER TABLE articlesource
ADD CONSTRAINT articlesource_article_id_fkey FOREIGN KEY (article_id) REFERENCES article (id);
