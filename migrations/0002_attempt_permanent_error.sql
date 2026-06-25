-- +goose Up
-- Extend the delivery_attempts.status domain with 'permanent_error' so that
-- permanent (non-retryable) failures are recorded distinctly from transient
-- 'error' attempts. The CHECK in 0001_init.sql is unnamed, so Postgres
-- auto-generated the name delivery_attempts_status_check.
ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_status_check;
ALTER TABLE delivery_attempts
    ADD CONSTRAINT delivery_attempts_status_check
    CHECK (status IN ('success', 'error', 'permanent_error'));

-- +goose Down
ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_status_check;
ALTER TABLE delivery_attempts
    ADD CONSTRAINT delivery_attempts_status_check
    CHECK (status IN ('success', 'error'));
