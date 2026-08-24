package store

import (
	"context"
	"database/sql"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) CreateDeployment(ctx context.Context, item domain.Deployment, job domain.Job, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO deployments(id,reservation_id,status,endpoint,version,started_at,stopped_at,created_at,updated_at) VALUES(?,?,?,?,?,NULL,NULL,?,?)`, item.ID, item.ReservationID, item.Status, item.Endpoint, item.Version, timestamp(item.CreatedAt), timestamp(item.UpdatedAt)); err != nil {
			return classify(err, "create deployment")
		}
		if err := insertJob(ctx, tx, job); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func scanDeployment(row interface{ Scan(...any) error }) (domain.Deployment, error) {
	var item domain.Deployment
	var started, stopped sql.NullString
	var created, updated string
	if err := row.Scan(&item.ID, &item.ReservationID, &item.Status, &item.Endpoint, &item.Version, &started, &stopped, &created, &updated); err != nil {
		return domain.Deployment{}, err
	}
	var err error
	if item.StartedAt, err = optionalTimestamp(started); err != nil {
		return domain.Deployment{}, err
	}
	if item.StoppedAt, err = optionalTimestamp(stopped); err != nil {
		return domain.Deployment{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.Deployment{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.Deployment{}, err
	}
	return item, nil
}

func (s *Store) FindDeployment(ctx context.Context, id string) (domain.Deployment, error) {
	item, err := scanDeployment(s.db.QueryRowContext(ctx, `SELECT id,reservation_id,status,endpoint,version,started_at,stopped_at,created_at,updated_at FROM deployments WHERE id=?`, id))
	return item, classify(err, "find deployment")
}

func (s *Store) FindDeploymentByReservation(ctx context.Context, reservationID string) (domain.Deployment, error) {
	item, err := scanDeployment(s.db.QueryRowContext(ctx, `SELECT id,reservation_id,status,endpoint,version,started_at,stopped_at,created_at,updated_at FROM deployments WHERE reservation_id=?`, reservationID))
	return item, classify(err, "find deployment by reservation")
}

func (s *Store) TransitionDeployment(ctx context.Context, item domain.Deployment, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := updateDeployment(ctx, tx, item); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func (s *Store) StopDeploymentAndRelease(ctx context.Context, deployment domain.Deployment, reservation domain.Reservation, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := updateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status=?,version=?,updated_at=? WHERE id=? AND version=? AND status='confirmed'`, reservation.Status, reservation.Version, timestamp(reservation.UpdatedAt), reservation.ID, reservation.Version-1)
		if err != nil {
			return classify(err, "release deployment reservation")
		}
		if err := requireChanged(result, domain.ErrVersion); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `UPDATE compute_sites SET available_units=available_units+?,version=version+1,updated_at=? WHERE id=? AND available_units+?<=total_units`, reservation.Units, timestamp(reservation.UpdatedAt), reservation.SiteID, reservation.Units)
		if err != nil {
			return classify(err, "return deployment capacity")
		}
		if err := requireChanged(result, domain.ErrConflict); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func (s *Store) PersistStoppedDeployment(ctx context.Context, deployment domain.Deployment) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		return updateDeployment(ctx, tx, deployment)
	})
}

func (s *Store) ReleaseStoppedDeploymentCapacity(ctx context.Context, reservation domain.Reservation, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status=?,version=?,updated_at=? WHERE id=? AND version=? AND status='confirmed'`, reservation.Status, reservation.Version, timestamp(reservation.UpdatedAt), reservation.ID, reservation.Version-1)
		if err != nil {
			return classify(err, "release deployment reservation")
		}
		if err := requireChanged(result, domain.ErrVersion); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `UPDATE compute_sites SET available_units=available_units+?,version=version+1,updated_at=? WHERE id=? AND available_units+?<=total_units`, reservation.Units, timestamp(reservation.UpdatedAt), reservation.SiteID, reservation.Units)
		if err != nil {
			return classify(err, "return deployment capacity")
		}
		if err := requireChanged(result, domain.ErrConflict); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func updateDeployment(ctx context.Context, tx *sql.Tx, item domain.Deployment) error {
	var started, stopped any
	if item.StartedAt != nil {
		started = timestamp(*item.StartedAt)
	}
	if item.StoppedAt != nil {
		stopped = timestamp(*item.StoppedAt)
	}
	result, err := tx.ExecContext(ctx, `UPDATE deployments SET status=?,endpoint=?,version=?,started_at=?,stopped_at=?,updated_at=? WHERE id=? AND version=?`, item.Status, item.Endpoint, item.Version, started, stopped, timestamp(item.UpdatedAt), item.ID, item.Version-1)
	if err != nil {
		return classify(err, "transition deployment")
	}
	return requireChanged(result, domain.ErrVersion)
}
