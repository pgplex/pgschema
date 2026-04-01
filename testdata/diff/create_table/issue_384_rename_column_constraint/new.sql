CREATE TABLE a (id bigint PRIMARY KEY, current_revision bigint);

CREATE TABLE b (id bigint PRIMARY KEY);

ALTER TABLE a ADD CONSTRAINT "a_revision_fkey" FOREIGN KEY ("current_revision") REFERENCES "b" ("id");
