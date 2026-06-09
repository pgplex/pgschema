CREATE TABLE public.user_sessions (
    user_id integer NOT NULL,
    session_token text NOT NULL,
    device_fingerprint text NOT NULL,
    created_at timestamp NOT NULL,
    CONSTRAINT user_sessions_token_device_key UNIQUE (session_token, device_fingerprint) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT user_sessions_user_device_key UNIQUE (user_id, device_fingerprint) DEFERRABLE INITIALLY DEFERRED
);