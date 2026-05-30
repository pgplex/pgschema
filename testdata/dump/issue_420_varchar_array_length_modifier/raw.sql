--
-- Test case for GitHub issue #420: varchar(n)[] length modifier silently dropped in dump
--
-- The length modifier is lost when dumping array columns of character types
-- with a length constraint: varchar(128)[] is emitted as varchar[].
--

CREATE TABLE items (
    id      integer PRIMARY KEY,
    name    varchar(64),
    tags    varchar(128)[],
    codes   character(10)[]
);
