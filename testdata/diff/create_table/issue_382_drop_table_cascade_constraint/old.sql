CREATE TABLE a (id bigint PRIMARY KEY, b_id bigint);

CREATE TABLE b (id bigint PRIMARY KEY);

ALTER TABLE a ADD CONSTRAINT "a_b_id_fkey" FOREIGN KEY ("b_id") REFERENCES "b" ("id");
