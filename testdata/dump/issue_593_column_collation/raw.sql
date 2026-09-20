CREATE TABLE recipient_search (
    id bigint PRIMARY KEY,
    search_text text COLLATE "C" NOT NULL,
    display_name varchar(100) COLLATE "POSIX" DEFAULT '',
    note text
);

CREATE INDEX recipient_search_prefix_idx
    ON recipient_search (search_text text_pattern_ops);
