package store

import (
	"context"
	"fmt"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func insertAudit(ctx context.Context, q querier, event domain.AuditEvent) error {
	_, err := q.ExecContext(ctx, `INSERT INTO audit_events(id,actor_id,action,object_type,object_id,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, event.ID, nullable(event.ActorID), event.Action, event.ObjectType, event.ObjectID, event.Result, event.RequestID, event.Details, timestamp(event.CreatedAt))
	return classify(err, "insert audit event")
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) ListAudit(ctx context.Context, objectType, objectID string, page domain.Page) ([]domain.AuditEvent, int, error) {
	page = page.Normalize()
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE object_type=? AND object_id=?`, objectType, objectID).Scan(&total); err != nil {
		return nil, 0, classify(err, "count audit events")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(actor_id,''),action,object_type,object_id,result,request_id,details,created_at FROM audit_events WHERE object_type=? AND object_id=? ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, objectType, objectID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, classify(err, "list audit events")
	}
	defer rows.Close()
	items := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var created string
		if err := rows.Scan(&event.ID, &event.ActorID, &event.Action, &event.ObjectType, &event.ObjectID, &event.Result, &event.RequestID, &event.Details, &created); err != nil {
			return nil, 0, fmt.Errorf("scan audit event: %w", err)
		}
		if event.CreatedAt, err = parseTime(created); err != nil {
			return nil, 0, err
		}
		items = append(items, event)
	}
	return items, total, rows.Err()
}
