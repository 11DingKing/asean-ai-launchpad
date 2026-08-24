package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
)

func TestPartnerScenarioListPreservesTenantScope(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "scenario-scope.db"))
	if err != nil {
		t.Fatalf("open durable store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	for _, user := range []domain.User{
		{ID: "partner_alpha", Email: "alpha@example.test", PasswordHash: "unused", Role: domain.RolePartner, Active: true, CreatedAt: now, UpdatedAt: now},
		{ID: "partner_beta", Email: "beta@example.test", PasswordHash: "unused", Role: domain.RolePartner, Active: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.CreateUser(ctx, user); err != nil {
			t.Fatalf("create partner %s: %v", user.ID, err)
		}
	}
	svc := New(database, clock.Fixed{Value: now}, &idgen.Sequence{}, time.Hour)
	alphaCtx := requestctx.WithPrincipal(ctx, requestctx.Principal{UserID: "partner_alpha", Role: string(domain.RolePartner)})
	betaCtx := requestctx.WithPrincipal(ctx, requestctx.Principal{UserID: "partner_beta", Role: string(domain.RolePartner)})
	if _, err := svc.SubmitScenario(alphaCtx, SubmitScenarioInput{Name: "Port inspection vision", Sector: "logistics", RequestedUnits: 4, DataClassification: "restricted"}); err != nil {
		t.Fatalf("submit alpha scenario: %v", err)
	}
	if _, err := svc.SubmitScenario(betaCtx, SubmitScenarioInput{Name: "Rice yield forecasting", Sector: "agriculture", RequestedUnits: 7, DataClassification: "sovereign"}); err != nil {
		t.Fatalf("submit beta scenario: %v", err)
	}

	firstPage, total, err := svc.ListScenarios(alphaCtx, domain.ScenarioSubmitted, domain.Page{Limit: 1})
	if err != nil {
		t.Fatalf("list alpha first page: %v", err)
	}
	secondPage, secondTotal, err := svc.ListScenarios(alphaCtx, domain.ScenarioSubmitted, domain.Page{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list alpha second page: %v", err)
	}
	if total != 1 || secondTotal != 1 || len(firstPage) != 1 || firstPage[0].PartnerID != "partner_alpha" || len(secondPage) != 0 {
		t.Fatalf("partner list crossed tenant boundary: first=%+v second=%+v totals=%d/%d", firstPage, secondPage, total, secondTotal)
	}

	operatorCtx := requestctx.WithPrincipal(ctx, requestctx.Principal{UserID: "operator_1", Role: string(domain.RoleOperator)})
	all, operatorTotal, err := svc.ListScenarios(operatorCtx, domain.ScenarioSubmitted, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("list operator scenarios: %v", err)
	}
	if operatorTotal != 2 || len(all) != 2 {
		t.Fatalf("operator global list was restricted: items=%+v total=%d", all, operatorTotal)
	}
}
