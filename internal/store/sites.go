package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) CreateSite(ctx context.Context, site domain.ComputeSite) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO compute_sites(id,operator_id,name,country_code,timezone,total_units,available_units,status,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, site.ID, site.OperatorID, site.Name, site.CountryCode, site.Timezone, site.TotalUnits, site.AvailableUnits, site.Status, site.Version, timestamp(site.CreatedAt), timestamp(site.UpdatedAt))
	return classify(err, "create site")
}

func scanSite(row interface{ Scan(...any) error }) (domain.ComputeSite, error) {
	var item domain.ComputeSite
	var created, updated string
	if err := row.Scan(&item.ID, &item.OperatorID, &item.Name, &item.CountryCode, &item.Timezone, &item.TotalUnits, &item.AvailableUnits, &item.Status, &item.Version, &created, &updated); err != nil {
		return domain.ComputeSite{}, err
	}
	var err error
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.ComputeSite{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ComputeSite{}, err
	}
	return item, nil
}

func (s *Store) FindSite(ctx context.Context, id string) (domain.ComputeSite, error) {
	item, err := scanSite(s.db.QueryRowContext(ctx, `SELECT id,operator_id,name,country_code,timezone,total_units,available_units,status,version,created_at,updated_at FROM compute_sites WHERE id=?`, id))
	return item, classify(err, "find site")
}

func (s *Store) ListSites(ctx context.Context, country string, status domain.SiteStatus, page domain.Page) ([]domain.ComputeSite, int, error) {
	page = page.Normalize()
	clauses := []string{"1=1"}
	args := []any{}
	if country != "" {
		clauses = append(clauses, "country_code=?")
		args = append(args, strings.ToUpper(country))
	}
	if status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, status)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM compute_sites WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, classify(err, "count sites")
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id,operator_id,name,country_code,timezone,total_units,available_units,status,version,created_at,updated_at FROM compute_sites WHERE `+where+` ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, classify(err, "list sites")
	}
	defer rows.Close()
	items := make([]domain.ComputeSite, 0)
	for rows.Next() {
		item, err := scanSite(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan site: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) ReviewSite(ctx context.Context, site domain.ComputeSite, review domain.ComplianceReview, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE compute_sites SET status=?,version=?,updated_at=? WHERE id=? AND version=? AND status='draft'`, site.Status, site.Version, timestamp(site.UpdatedAt), site.ID, site.Version-1)
		if err != nil {
			return classify(err, "review site")
		}
		if err := requireChanged(result, domain.ErrVersion); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_reviews(id,site_id,reviewer_id,decision,evidence_ref,created_at) VALUES(?,?,?,?,?,?)`, review.ID, review.SiteID, review.ReviewerID, review.Decision, review.EvidenceRef, timestamp(review.CreatedAt)); err != nil {
			return classify(err, "record compliance review")
		}
		return insertAudit(ctx, tx, audit)
	})
}

func (s *Store) TransitionSite(ctx context.Context, site domain.ComputeSite, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE compute_sites SET status=?,version=?,updated_at=? WHERE id=? AND version=?`, site.Status, site.Version, timestamp(site.UpdatedAt), site.ID, site.Version-1)
		if err != nil {
			return classify(err, "transition site")
		}
		if err := requireChanged(result, domain.ErrVersion); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}
