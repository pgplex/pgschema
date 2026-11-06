-- Every option that is not a function: a non-default operator class, a
-- collation that differs from the subtype's, and an explicit multirange name.
CREATE TYPE textrange AS RANGE (
    subtype = text,
    subtype_opclass = text_pattern_ops,
    collation = "C",
    multirange_type_name = textspans
);

-- Default multirange names are not printed: "range" inside the name becomes
-- "multirange" (pricemultirange), and a name without "range" gets
-- "_multirange" appended (span_multirange).
CREATE TYPE pricerange AS RANGE (subtype = numeric);

CREATE TYPE span AS RANGE (subtype = integer);
