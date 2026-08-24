package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) CreateReservation(ctx context.Context, item domain.Reservation, job domain.Job, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE compute_sites SET available_units=available_units-?,version=version+1,updated_at=? WHERE id=? AND status='active' AND available_units>=?`, item.Units, timestamp(item.CreatedAt), item.SiteID, item.Units)
		if err != nil {
			return classify(err, "claim site capacity")
		}
		if err := requireChanged(result, domain.ErrCapacity); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO reservations(id,scenario_id,site_id,partner_id,units,status,expires_at,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ScenarioID, item.SiteID, item.PartnerID, item.Units, item.Status, timestamp(item.ExpiresAt), item.Version, timestamp(item.CreatedAt), timestamp(item.UpdatedAt)); err != nil {
			return classify(err, "insert reservation")
		}
		if err := insertJob(ctx, tx, job); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func scanReservation(row interface{ Scan(...any) error }) (domain.Reservation, error) {
	var item domain.Reservation
	var expires, created, updated string
	if err := row.Scan(&item.ID, &item.ScenarioID, &item.SiteID, &item.PartnerID, &item.Units, &item.Status, &expires, &item.Version, &created, &updated); err != nil {
		return domain.Reservation{}, err
	}
	var err error
	if item.ExpiresAt, err = parseTime(expires); err != nil {
		return domain.Reservation{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.Reservation{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.Reservation{}, err
	}
	return item, nil
}

func (s *Store) FindReservation(ctx context.Context, id string) (domain.Reservation, error) {
	item, err := scanReservation(s.db.QueryRowContext(ctx, `SELECT id,scenario_id,site_id,partner_id,units,status,expires_at,version,created_at,updated_at FROM reservations WHERE id=?`, id))
	return item, classify(err, "find reservation")
}

func (s *Store) ReleaseReservation(ctx context.Context, item domain.Reservation, audit domain.AuditEvent) error {
	return s.changeReservationAndReturnCapacity(ctx, item, audit, "release reservation")
}

func (s *Store) ExpireReservation(ctx context.Context, item domain.Reservation, audit domain.AuditEvent) error {
	return s.changeReservationAndReturnCapacity(ctx, item, audit, "expire reservation")
}

func (s *Store) changeReservationAndReturnCapacity(ctx context.Context, item domain.Reservation, audit domain.AuditEvent, action string) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status=?,version=?,updated_at=? WHERE id=? AND version=? AND status IN ('pending','confirmed')`, item.Status, item.Version, timestamp(item.UpdatedAt), item.ID, item.Version-1)
		if err != nil {
			return classify(err, action)
		}
		if err := requireChanged(result, domain.ErrVersion); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `UPDATE compute_sites SET available_units=available_units+?,version=version+1,updated_at=? WHERE id=? AND available_units+?<=total_units`, item.Units, timestamp(item.UpdatedAt), item.SiteID, item.Units)
		if err != nil {
			return classify(err, "return site capacity")
		}
		if err := requireChanged(result, fmt.Errorf("%w: capacity return invariant", domain.ErrConflict)); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}
