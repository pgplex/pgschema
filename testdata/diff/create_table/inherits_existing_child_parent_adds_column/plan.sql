ALTER TABLE events ADD COLUMN priority integer DEFAULT 1;

ALTER TABLE alerts ADD CONSTRAINT alerts_priority_not_null NOT NULL priority NOT VALID;

ALTER TABLE alerts VALIDATE CONSTRAINT alerts_priority_not_null;

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
