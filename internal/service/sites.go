package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

type CreateSiteInput struct {
	Name        string `json:"name"`
	CountryCode string `json:"country_code"`
	Timezone    string `json:"timezone"`
	TotalUnits  int    `json:"total_units"`
}

func (s *Service) CreateSite(ctx context.Context, input CreateSiteInput) (domain.ComputeSite, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	id, err := s.IDs.New("site")
	if err != nil {
		return domain.ComputeSite{}, err
	}
	item, err := domain.NewComputeSite(id, actor.UserID, input.Name, input.CountryCode, input.Timezone, input.TotalUnits, s.Clock.Now())
	if err != nil {
		return domain.ComputeSite{}, err
	}
	if err := s.Store.CreateSite(ctx, item); err != nil {
		return domain.ComputeSite{}, err
	}
	return item, nil
}

func (s *Service) ReviewSite(ctx context.Context, id, decision, evidence string, version int64) (domain.ComputeSite, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	evidence, err = requireText(evidence, "evidence_ref", 4, 500)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	item, err := s.Store.FindSite(ctx, id)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	if item.Version != version {
		return domain.ComputeSite{}, domain.ErrVersion
	}
	if decision != "approved" && decision != "rejected" {
		return domain.ComputeSite{}, fmt.Errorf("%w: review decision", domain.ErrInvalid)
	}
	next := domain.SiteVerified
	if decision == "rejected" {
		next = domain.SiteSuspended
	}
	if err := item.Transition(next, s.Clock.Now()); err != nil {
		return domain.ComputeSite{}, err
	}
	reviewID, err := s.IDs.New("srev")
	if err != nil {
		return domain.ComputeSite{}, err
	}
	review := domain.ComplianceReview{ID: reviewID, SiteID: item.ID, ReviewerID: actor.UserID, Decision: decision, EvidenceRef: evidence, CreatedAt: s.Clock.Now()}
	event, err := s.event(ctx, actor.UserID, "site.review", "compute_site", item.ID, decision, map[string]any{"version": version, "evidence_ref": evidence})
	if err != nil {
		return domain.ComputeSite{}, err
	}
	if err := s.Store.ReviewSite(ctx, item, review, event); err != nil {
		return domain.ComputeSite{}, err
	}
	return item, nil
}

func (s *Service) TransitionSite(ctx context.Context, id string, next domain.SiteStatus, version int64) (domain.ComputeSite, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	item, err := s.Store.FindSite(ctx, id)
	if err != nil {
		return domain.ComputeSite{}, err
	}
	if item.OperatorID != actor.UserID {
		return domain.ComputeSite{}, domain.ErrForbidden
	}
	if item.Version != version {
		return domain.ComputeSite{}, domain.ErrVersion
	}
	if err := item.Transition(next, s.Clock.Now()); err != nil {
		return domain.ComputeSite{}, err
	}
	event, err := s.event(ctx, actor.UserID, "site.transition", "compute_site", item.ID, "success", map[string]any{"status": next, "version": version})
	if err != nil {
		return domain.ComputeSite{}, err
	}
	if err := s.Store.TransitionSite(ctx, item, event); err != nil {
		return domain.ComputeSite{}, err
	}
	return item, nil
}

func (s *Service) ListSites(ctx context.Context, country string, status domain.SiteStatus, page domain.Page) ([]domain.ComputeSite, int, error) {
	if _, err := requireRole(ctx, domain.RoleOperator, domain.RolePartner); err != nil {
		return nil, 0, err
	}
	return s.Store.ListSites(ctx, strings.TrimSpace(country), status, page)
}
