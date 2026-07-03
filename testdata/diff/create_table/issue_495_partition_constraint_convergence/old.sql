-- This represents what the target database looks like after pgschema apply:
-- partition children created as standalone tables with explicit constraints,
-- then attached to the parent table.
CREATE TABLE public.p (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_band_id_geom_id_key UNIQUE (band_id, geom_id)
) PARTITION BY LIST (band_id);

CREATE TABLE public.p_12 (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_12_band_id_geom_id_key UNIQUE (band_id, geom_id)
);
ALTER TABLE public.p ATTACH PARTITION public.p_12 FOR VALUES IN (12);

CREATE TABLE public.p_13 (
    band_id integer NOT NULL,
    geom_id integer NOT NULL,
    CONSTRAINT p_13_band_id_geom_id_key UNIQUE (band_id, geom_id)
);
ALTER TABLE public.p ATTACH PARTITION public.p_13 FOR VALUES IN (13);
