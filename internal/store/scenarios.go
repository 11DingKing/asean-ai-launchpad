package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) CreateScenario(ctx context.Context, item domain.Scenario) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO scenarios(id,partner_id,name,sector,requested_units,data_classification,status,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.PartnerID, item.Name, item.Sector, item.RequestedUnits, item.DataClassification, item.Status, item.Version, timestamp(item.CreatedAt), timestamp(item.UpdatedAt))
	return classify(err, "create scenario")
}

func scanScenario(row interface{ Scan(...any) error }) (domain.Scenario, error) {
	var item domain.Scenario
	var created, updated string
	if err := row.Scan(&item.ID, &item.PartnerID, &item.Name, &item.Sector, &item.RequestedUnits, &item.DataClassification, &item.Status, &item.Version, &created, &updated); err != nil {
		return domain.Scenario{}, err
	}
	var err error
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.Scenario{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.Scenario{}, err
	}
	return item, nil
}

func (s *Store) FindScenario(ctx context.Context, id string) (domain.Scenario, error) {
	item, err := scanScenario(s.db.QueryRowContext(ctx, `SELECT id,partner_id,name,sector,requested_units,data_classification,status,version,created_at,updated_at FROM scenarios WHERE id=?`, id))
	return item, classify(err, "find scenario")
}

func (s *Store) ListScenarios(ctx context.Context, partnerID string, status domain.ScenarioStatus, page domain.Page) ([]domain.Scenario, int, error) {
	page = page.Normalize()
	clauses := []string{"1=1"}
	args := []any{}
	if partnerID != "" {
		clauses = append(clauses, "partner_id=?")
		args = append(args, partnerID)
	}
	if status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, status)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scenarios WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, classify(err, "count scenarios")
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id,partner_id,name,sector,requested_units,data_classification,status,version,created_at,updated_at FROM scenarios WHERE `+where+` ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, classify(err, "list scenarios")
	}
	defer rows.Close()
	items := make([]domain.Scenario, 0)
	for rows.Next() {
		item, err := scanScenario(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan scenario: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) StartScenarioReview(ctx context.Context, item domain.Scenario, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := updateScenario(ctx, tx, item); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}

func (s *Store) DecideScenario(ctx context.Context, item domain.Scenario, review domain.ScenarioReview, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := updateScenario(ctx, tx, item); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scenario_reviews(id,scenario_id,reviewer_id,decision,notes,created_at) VALUES(?,?,?,?,?,?)`, review.ID, review.ScenarioID, review.ReviewerID, review.Decision, review.Notes, timestamp(review.CreatedAt)); err != nil {
			return classify(err, "record scenario review")
		}
		return insertAudit(ctx, tx, audit)
	})
}

func updateScenario(ctx context.Context, tx *sql.Tx, item domain.Scenario) error {
	result, err := tx.ExecContext(ctx, `UPDATE scenarios SET status=?,version=?,updated_at=? WHERE id=? AND version=?`, item.Status, item.Version, timestamp(item.UpdatedAt), item.ID, item.Version-1)
	if err != nil {
		return classify(err, "update scenario")
	}
	return requireChanged(result, domain.ErrVersion)
}
