CREATE TABLE t (
    id integer NOT NULL,
    a integer,
    b integer,
    CONSTRAINT t_pk PRIMARY KEY (id)
);

CREATE INDEX t_cov ON t (a) INCLUDE (b);
