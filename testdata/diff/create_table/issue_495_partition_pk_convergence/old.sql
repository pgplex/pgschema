CREATE TABLE public.p (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_pkey PRIMARY KEY (band_id, geom_id)
) PARTITION BY LIST (band_id);

CREATE TABLE public.p_12 (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_12_pkey PRIMARY KEY (band_id, geom_id)
);
ALTER TABLE public.p ATTACH PARTITION public.p_12 FOR VALUES IN (12);

CREATE TABLE public.p_13 (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_13_pkey PRIMARY KEY (band_id, geom_id)
);
ALTER TABLE public.p ATTACH PARTITION public.p_13 FOR VALUES IN (13);
