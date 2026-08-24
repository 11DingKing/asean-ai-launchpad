package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) RecordUsageAndSettlement(ctx context.Context, usage domain.UsageRecord, settlement domain.SettlementEntry, audit domain.AuditEvent) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_records(id,deployment_id,partner_id,period_start,period_end,unit_seconds,created_at) VALUES(?,?,?,?,?,?,?)`, usage.ID, usage.DeploymentID, usage.PartnerID, timestamp(usage.PeriodStart), timestamp(usage.PeriodEnd), usage.UnitSeconds, timestamp(usage.CreatedAt)); err != nil {
			return classify(err, "record usage")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO settlement_entries(id,usage_id,partner_id,amount_micros,currency,created_at) VALUES(?,?,?,?,?,?)`, settlement.ID, settlement.UsageID, settlement.PartnerID, settlement.AmountMicros, settlement.Currency, timestamp(settlement.CreatedAt)); err != nil {
			return classify(err, "record settlement")
		}
		return insertAudit(ctx, tx, audit)
	})
}

func (s *Store) ListUsage(ctx context.Context, partnerID string, from, to time.Time, page domain.Page) ([]domain.UsageRecord, int, error) {
	page = page.Normalize()
	args := []any{partnerID, timestamp(from), timestamp(to)}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_records WHERE partner_id=? AND period_start>=? AND period_end<=?`, args...).Scan(&total); err != nil {
		return nil, 0, classify(err, "count usage")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,deployment_id,partner_id,period_start,period_end,unit_seconds,created_at FROM usage_records WHERE partner_id=? AND period_start>=? AND period_end<=? ORDER BY period_start DESC,id LIMIT ? OFFSET ?`, partnerID, timestamp(from), timestamp(to), page.Limit, page.Offset)
	if err != nil {
		return nil, 0, classify(err, "list usage")
	}
	defer rows.Close()
	items := make([]domain.UsageRecord, 0)
	for rows.Next() {
		var item domain.UsageRecord
		var start, end, created string
		if err := rows.Scan(&item.ID, &item.DeploymentID, &item.PartnerID, &start, &end, &item.UnitSeconds, &created); err != nil {
			return nil, 0, fmt.Errorf("scan usage: %w", err)
		}
		var parseErr error
		if item.PeriodStart, parseErr = parseTime(start); parseErr != nil {
			return nil, 0, parseErr
		}
		if item.PeriodEnd, parseErr = parseTime(end); parseErr != nil {
			return nil, 0, parseErr
		}
		if item.CreatedAt, parseErr = parseTime(created); parseErr != nil {
			return nil, 0, parseErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
