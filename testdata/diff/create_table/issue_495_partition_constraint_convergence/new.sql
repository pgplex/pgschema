CREATE TABLE public.p (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_band_id_geom_id_key UNIQUE (band_id, geom_id)
) PARTITION BY LIST (band_id);

CREATE TABLE public.p_12 PARTITION OF public.p FOR VALUES IN (12);
CREATE TABLE public.p_13 PARTITION OF public.p FOR VALUES IN (13);
