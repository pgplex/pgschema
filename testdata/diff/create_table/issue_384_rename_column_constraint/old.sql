CREATE TABLE a (id bigint PRIMARY KEY, revision bigint);

CREATE TABLE b (id bigint PRIMARY KEY);

ALTER TABLE a ADD CONSTRAINT "a_revision_fkey" FOREIGN KEY ("revision") REFERENCES "b" ("id");
