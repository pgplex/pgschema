ALTER TABLE code ADD COLUMN a varchar(20);
ALTER TABLE code ADD COLUMN b varchar(20);
ALTER TABLE code
ADD CONSTRAINT code_a_check CHECK (a IS NULL OR a::text LIKE 'p-%' AND b IS NOT NULL);
ALTER TABLE code
ADD CONSTRAINT code_check CHECK (code > 0 AND code < 255);
