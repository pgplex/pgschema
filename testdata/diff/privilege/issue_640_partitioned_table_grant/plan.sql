REVOKE INSERT ON TABLE events FROM reader;

GRANT SELECT ON TABLE events TO reader;

GRANT UPDATE (at) ON TABLE events TO reader;
