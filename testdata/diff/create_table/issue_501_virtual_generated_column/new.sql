CREATE TABLE public.vt (
    slug text NOT NULL,
    identifier text GENERATED ALWAYS AS ('urn:sdp:catalog:' || slug) VIRTUAL
);
