CREATE TYPE pricerange AS RANGE (
    subtype = numeric
);

CREATE TYPE span AS RANGE (
    subtype = integer
);

CREATE TYPE textrange AS RANGE (
    subtype = text,
    multirange_type_name = textspans,
    subtype_opclass = pg_catalog.text_pattern_ops,
    collation = "C"
);
