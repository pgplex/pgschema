CREATE TYPE public.user_status_log_source_enum AS ENUM ('web', 'api', 'mobile');

CREATE TYPE public.document_document_type_enum AS ENUM ('invoice', 'receipt', 'contract');

CREATE TABLE public.user_status_log (
    id integer NOT NULL,
    source public.user_status_log_source_enum NOT NULL
);

CREATE TABLE public.document (
    id integer NOT NULL,
    document_type public.document_document_type_enum NOT NULL
);
