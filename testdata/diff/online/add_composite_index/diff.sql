CREATE INDEX IF NOT EXISTS idx_users_email_include ON users (email) INCLUDE (username);

CREATE INDEX IF NOT EXISTS idx_users_email_status ON users (email, status DESC);
