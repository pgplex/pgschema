ALTER TABLE nr_cell_du ALTER COLUMN arfcn_dl TYPE integer USING arfcn_dl::integer;

ALTER TABLE nr_cell_du ALTER COLUMN priority DROP DEFAULT;

ALTER TABLE nr_cell_du ALTER COLUMN priority TYPE integer USING priority::integer;

ALTER TABLE nr_cell_du ALTER COLUMN priority SET DEFAULT 0;
