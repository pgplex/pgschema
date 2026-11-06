DROP TYPE IF EXISTS positive_span RESTRICT;

DROP FUNCTION IF EXISTS positive_diff(positive, positive);

DROP DOMAIN IF EXISTS positive RESTRICT;

DROP FUNCTION IF EXISTS is_positive(integer);
