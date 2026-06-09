--
-- Test case for GitHub issue #421: Quoted mixed-case FK columns cannot be round-tripped
--
-- A schema with quoted mixed-case column names and circular foreign key
-- dependencies must round-trip through dump + plan. Both the referencing and
-- referenced columns of every foreign key must remain double-quoted, otherwise
-- the dumped SQL folds them to lower case and the referenced column "does not exist".
--

CREATE TABLE aaa (
    "aId" bigint NOT NULL,
    "bId" bigint,
    CONSTRAINT aaa_pkey PRIMARY KEY ("aId")
);

CREATE TABLE bbb (
    "bId" bigint NOT NULL,
    "aId" bigint,
    CONSTRAINT bbb_pkey PRIMARY KEY ("bId")
);

ALTER TABLE aaa ADD CONSTRAINT aaa_fk FOREIGN KEY ("bId") REFERENCES bbb ("bId") DEFERRABLE;
ALTER TABLE bbb ADD CONSTRAINT bbb_fk FOREIGN KEY ("aId") REFERENCES aaa ("aId") DEFERRABLE;
