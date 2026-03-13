CREATE TABLE IF NOT EXISTS documents (
    id SERIAL,
    title text NOT NULL,
    content text,
    created_at timestamp DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT documents_pkey PRIMARY KEY (id)
);

CREATE OR REPLACE FUNCTION get_document_count()
RETURNS integer
LANGUAGE plpgsql

VOLATILE
AS $$
BEGIN
    RETURN (SELECT COUNT(*) FROM public.documents);
END;
$$;
