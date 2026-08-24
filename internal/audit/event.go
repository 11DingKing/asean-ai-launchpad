package audit

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
)

type Factory struct {
	IDs idgen.Generator
}

func (f Factory) Event(actorID, action, objectType, objectID, result, requestID string, details any, now time.Time) (domain.AuditEvent, error) {
	id, err := f.IDs.New("audit")
	if err != nil {
		return domain.AuditEvent{}, err
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("encode audit details: %w", err)
	}
	return domain.AuditEvent{ID: id, ActorID: actorID, Action: action, ObjectType: objectType, ObjectID: objectID, Result: result, RequestID: requestID, Details: string(encoded), CreatedAt: now.UTC()}, nil
}
