DROP TABLE IF EXISTS shifts CASCADE;

DROP TYPE IF EXISTS timerange RESTRICT;

DROP FUNCTION IF EXISTS time_diff(time without time zone, time without time zone);
