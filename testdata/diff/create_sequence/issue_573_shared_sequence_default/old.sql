-- Existing table that gains a column backed by a custom-named owned sequence
CREATE TABLE legacy (
    id bigint NOT NULL,
    CONSTRAINT legacy_pkey PRIMARY KEY (id)
);
