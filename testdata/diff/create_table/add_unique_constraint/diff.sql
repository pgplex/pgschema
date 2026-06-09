ALTER TABLE user_sessions
ADD CONSTRAINT user_sessions_token_device_key UNIQUE (session_token, device_fingerprint) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE user_sessions DROP CONSTRAINT user_sessions_user_device_key;

ALTER TABLE user_sessions
ADD CONSTRAINT user_sessions_user_device_key UNIQUE (user_id, device_fingerprint) DEFERRABLE INITIALLY DEFERRED;