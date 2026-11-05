ALTER TABLE measurements ALTER COLUMN scaled SET EXPRESSION AS ((raw * 4));

ALTER TABLE ONLY calibrated ALTER COLUMN scaled SET EXPRESSION AS ((raw * 3));
