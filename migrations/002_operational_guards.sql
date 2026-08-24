CREATE TRIGGER reservations_units_guard_insert
BEFORE INSERT ON reservations
WHEN NEW.units <= 0
BEGIN
    SELECT RAISE(ABORT, 'reservation units must be positive');
END;

CREATE TRIGGER usage_period_guard_insert
BEFORE INSERT ON usage_records
WHEN NEW.period_end <= NEW.period_start
BEGIN
    SELECT RAISE(ABORT, 'usage period must end after it starts');
END;

CREATE INDEX deployments_status_updated_idx ON deployments(status, updated_at);
CREATE INDEX usage_partner_period_idx ON usage_records(partner_id, period_start, period_end);
