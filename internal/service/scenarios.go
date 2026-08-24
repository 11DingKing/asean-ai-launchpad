package service

import (
	"context"
	"fmt"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

type SubmitScenarioInput struct {
	Name               string `json:"name"`
	Sector             string `json:"sector"`
	RequestedUnits     int    `json:"requested_units"`
	DataClassification string `json:"data_classification"`
}

func (s *Service) SubmitScenario(ctx context.Context, input SubmitScenarioInput) (domain.Scenario, error) {
	actor, err := requireRole(ctx, domain.RolePartner)
	if err != nil {
		return domain.Scenario{}, err
	}
	id, err := s.IDs.New("scn")
	if err != nil {
		return domain.Scenario{}, err
	}
	item, err := domain.NewScenario(id, actor.UserID, input.Name, input.Sector, input.DataClassification, input.RequestedUnits, s.Clock.Now())
	if err != nil {
		return domain.Scenario{}, err
	}
	if err := s.Store.CreateScenario(ctx, item); err != nil {
		return domain.Scenario{}, err
	}
	return item, nil
}

func (s *Service) StartScenarioReview(ctx context.Context, id string, version int64) (domain.Scenario, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.Scenario{}, err
	}
	item, err := s.Store.FindScenario(ctx, id)
	if err != nil {
		return domain.Scenario{}, err
	}
	if item.Version != version {
		return domain.Scenario{}, domain.ErrVersion
	}
	if err := item.Transition(domain.ScenarioReviewing, s.Clock.Now()); err != nil {
		return domain.Scenario{}, err
	}
	event, err := s.event(ctx, actor.UserID, "scenario.review.start", "scenario", item.ID, "success", map[string]any{"version": version})
	if err != nil {
		return domain.Scenario{}, err
	}
	if err := s.Store.StartScenarioReview(ctx, item, event); err != nil {
		return domain.Scenario{}, err
	}
	return item, nil
}

func (s *Service) DecideScenario(ctx context.Context, id, decision, notes string, version int64) (domain.Scenario, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.Scenario{}, err
	}
	notes, err = requireText(notes, "review notes", 3, 1000)
	if err != nil {
		return domain.Scenario{}, err
	}
	item, err := s.Store.FindScenario(ctx, id)
	if err != nil {
		return domain.Scenario{}, err
	}
	if item.Version != version {
		return domain.Scenario{}, domain.ErrVersion
	}
	var next domain.ScenarioStatus
	switch decision {
	case "approved":
		next = domain.ScenarioApproved
	case "rejected":
		next = domain.ScenarioRejected
	default:
		return domain.Scenario{}, fmt.Errorf("%w: review decision", domain.ErrInvalid)
	}
	if err := item.Transition(next, s.Clock.Now()); err != nil {
		return domain.Scenario{}, err
	}
	reviewID, err := s.IDs.New("crev")
	if err != nil {
		return domain.Scenario{}, err
	}
	review := domain.ScenarioReview{ID: reviewID, ScenarioID: item.ID, ReviewerID: actor.UserID, Decision: decision, Notes: notes, CreatedAt: s.Clock.Now()}
	event, err := s.event(ctx, actor.UserID, "scenario.review.decide", "scenario", item.ID, decision, map[string]any{"version": version, "notes": notes})
	if err != nil {
		return domain.Scenario{}, err
	}
	if err := s.Store.DecideScenario(ctx, item, review, event); err != nil {
		return domain.Scenario{}, err
	}
	return item, nil
}

func (s *Service) ListScenarios(ctx context.Context, status domain.ScenarioStatus, page domain.Page) ([]domain.Scenario, int, error) {
	if _, err := requireRole(ctx, domain.RoleOperator, domain.RolePartner); err != nil {
		return nil, 0, err
	}
	return s.Store.ListScenarios(ctx, status, page)
}
