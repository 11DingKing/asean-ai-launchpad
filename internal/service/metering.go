package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

const rateMicrosPerUnitSecond int64 = 17

func (s *Service) RecordUsage(ctx context.Context, deploymentID, partnerID string, start, end time.Time, unitSeconds int64, currency string) (domain.UsageRecord, domain.SettlementEntry, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	deployment, err := s.Store.FindDeployment(ctx, deploymentID)
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	if deployment.Status != domain.DeploymentRunning && deployment.Status != domain.DeploymentStopped {
		return domain.UsageRecord{}, domain.SettlementEntry{}, fmt.Errorf("%w: deployment not meterable", domain.ErrIllegalState)
	}
	reservation, err := s.Store.FindReservation(ctx, deployment.ReservationID)
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	if reservation.PartnerID != partnerID {
		return domain.UsageRecord{}, domain.SettlementEntry{}, domain.ErrForbidden
	}
	usageID, err := s.IDs.New("use")
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	usage, err := domain.NewUsageRecord(usageID, deploymentID, partnerID, start, end, unitSeconds, s.Clock.Now())
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	settlementID, err := s.IDs.New("set")
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	if unitSeconds > (1<<63-1)/rateMicrosPerUnitSecond {
		return domain.UsageRecord{}, domain.SettlementEntry{}, fmt.Errorf("%w: settlement overflow", domain.ErrInvalid)
	}
	settlement := domain.SettlementEntry{ID: settlementID, UsageID: usage.ID, PartnerID: partnerID, AmountMicros: unitSeconds * rateMicrosPerUnitSecond, Currency: currency, CreatedAt: s.Clock.Now()}
	event, err := s.event(ctx, actor.UserID, "usage.record", "deployment", deploymentID, "success", map[string]any{"usage_id": usage.ID, "amount_micros": settlement.AmountMicros})
	if err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	if err := s.Store.RecordUsage(ctx, usage); err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	if err := s.Store.RecordSettlement(ctx, settlement, event); err != nil {
		return domain.UsageRecord{}, domain.SettlementEntry{}, err
	}
	return usage, settlement, nil
}

func (s *Service) ListUsage(ctx context.Context, from, to time.Time, page domain.Page) ([]domain.UsageRecord, int, error) {
	actor, err := requireRole(ctx, domain.RolePartner)
	if err != nil {
		return nil, 0, err
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, 0, fmt.Errorf("%w: usage window", domain.ErrInvalid)
	}
	return s.Store.ListUsage(ctx, actor.UserID, from, to, page)
}
