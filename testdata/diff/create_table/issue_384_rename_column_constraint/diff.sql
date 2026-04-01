ALTER TABLE a DROP COLUMN revision;

ALTER TABLE a ADD COLUMN current_revision bigint;

ALTER TABLE a
ADD CONSTRAINT a_revision_fkey FOREIGN KEY (current_revision) REFERENCES b (id);
