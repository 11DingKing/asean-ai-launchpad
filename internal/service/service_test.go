package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
)

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type serviceFixture struct {
	service  *Service
	store    *store.Store
	clock    *mutableClock
	operator domain.User
	partner  domain.User
	opCtx    context.Context
	partCtx  context.Context
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clk := &mutableClock{now: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)}
	ids := &idgen.Sequence{}
	svc := New(database, clk, ids, time.Hour)
	operator, err := svc.BootstrapOperator(context.Background(), "operator@example.test", "operator-password-123")
	if err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	partner, err := svc.RegisterPartner(context.Background(), "partner@example.test", "partner-password-123")
	if err != nil {
		t.Fatalf("register partner: %v", err)
	}
	opCtx := requestctx.WithRequestID(context.Background(), "req_operator")
	opCtx = requestctx.WithPrincipal(opCtx, requestctx.Principal{UserID: operator.ID, Role: string(operator.Role)})
	partCtx := requestctx.WithRequestID(context.Background(), "req_partner")
	partCtx = requestctx.WithPrincipal(partCtx, requestctx.Principal{UserID: partner.ID, Role: string(partner.Role)})
	return &serviceFixture{service: svc, store: database, clock: clk, operator: operator, partner: partner, opCtx: opCtx, partCtx: partCtx}
}

func (f *serviceFixture) activeSite(t *testing.T, units int) domain.ComputeSite {
	t.Helper()
	site, err := f.service.CreateSite(f.opCtx, CreateSiteInput{Name: "Bangkok AI Corridor", CountryCode: "TH", Timezone: "Asia/Bangkok", TotalUnits: units})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	site, err = f.service.ReviewSite(f.opCtx, site.ID, "approved", "evidence://regional-compliance", site.Version)
	if err != nil {
		t.Fatalf("review site: %v", err)
	}
	site, err = f.service.TransitionSite(f.opCtx, site.ID, domain.SiteActive, site.Version)
	if err != nil {
		t.Fatalf("activate site: %v", err)
	}
	return site
}

func (f *serviceFixture) approvedScenario(t *testing.T, units int) domain.Scenario {
	t.Helper()
	scenario, err := f.service.SubmitScenario(f.partCtx, SubmitScenarioInput{Name: "Regional document intelligence", Sector: "public services", RequestedUnits: units, DataClassification: "restricted"})
	if err != nil {
		t.Fatalf("submit scenario: %v", err)
	}
	scenario, err = f.service.StartScenarioReview(f.opCtx, scenario.ID, scenario.Version)
	if err != nil {
		t.Fatalf("start review: %v", err)
	}
	scenario, err = f.service.DecideScenario(f.opCtx, scenario.ID, "approved", "Meets sovereign data requirements", scenario.Version)
	if err != nil {
		t.Fatalf("approve scenario: %v", err)
	}
	return scenario
}

func TestAuthenticationLoginLogoutAndExpiry(t *testing.T) {
	fixture := newServiceFixture(t)
	result, err := fixture.service.Login(context.Background(), " PARTNER@example.test ", "partner-password-123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Token == "" || result.User.ID != fixture.partner.ID {
		t.Fatalf("login result: %+v", result)
	}
	principal, sessionID, err := fixture.service.Authenticate(context.Background(), result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.UserID != fixture.partner.ID || principal.Role != string(domain.RolePartner) || sessionID == "" {
		t.Fatalf("principal mismatch: %+v session=%s", principal, sessionID)
	}
	authCtx := requestctx.WithPrincipal(context.Background(), principal)
	if err := fixture.service.Logout(authCtx, sessionID); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := fixture.service.Authenticate(context.Background(), result.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked token should fail: %v", err)
	}
	second, err := fixture.service.Login(context.Background(), fixture.partner.Email, "partner-password-123")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	fixture.clock.Advance(time.Hour)
	if _, _, err := fixture.service.Authenticate(context.Background(), second.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token should fail: %v", err)
	}
	if _, err := fixture.service.Login(context.Background(), fixture.partner.Email, "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password should fail: %v", err)
	}
}

func TestRoleAuthorizationAcrossBusinessServices(t *testing.T) {
	fixture := newServiceFixture(t)
	if _, err := fixture.service.CreateSite(fixture.partCtx, CreateSiteInput{Name: "Partner Site", CountryCode: "SG", Timezone: "Asia/Singapore", TotalUnits: 2}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner created site: %v", err)
	}
	if _, err := fixture.service.SubmitScenario(fixture.opCtx, SubmitScenarioInput{Name: "Operator Scenario", Sector: "energy", RequestedUnits: 1, DataClassification: "public"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("operator submitted partner scenario: %v", err)
	}
	if _, _, err := fixture.service.ListUsage(fixture.opCtx, fixture.clock.Now().Add(-time.Hour), fixture.clock.Now(), domain.Page{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("operator read partner usage endpoint: %v", err)
	}
	if _, err := fixture.service.CreateSite(context.Background(), CreateSiteInput{}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("anonymous create site error=%v", err)
	}
}

func TestEndToEndLaunchpadLifecycle(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 12)
	scenario := fixture.approvedScenario(t, 4)
	reservation, err := fixture.service.ReserveCapacity(fixture.partCtx, scenario.ID, site.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("reserve capacity: %v", err)
	}
	persistedSite, err := fixture.store.FindSite(context.Background(), site.ID)
	if err != nil || persistedSite.AvailableUnits != 8 {
		t.Fatalf("claimed capacity=%+v err=%v", persistedSite, err)
	}
	deployment, err := fixture.service.CreateDeployment(fixture.partCtx, reservation.ID)
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := fixture.service.ActivateDeploymentJob(context.Background(), deployment.ID); err != nil {
		t.Fatalf("activate deployment job: %v", err)
	}
	deployment, err = fixture.store.FindDeployment(context.Background(), deployment.ID)
	if err != nil || deployment.Status != domain.DeploymentRunning || deployment.Endpoint == "" {
		t.Fatalf("running deployment=%+v err=%v", deployment, err)
	}
	start := fixture.clock.Now().Add(-time.Hour)
	usage, settlement, err := fixture.service.RecordUsage(fixture.opCtx, deployment.ID, fixture.partner.ID, start, fixture.clock.Now(), 14_400, "USD")
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if settlement.UsageID != usage.ID || settlement.AmountMicros != 244_800 {
		t.Fatalf("settlement mismatch: %+v usage=%+v", settlement, usage)
	}
	items, total, err := fixture.service.ListUsage(fixture.partCtx, start.Add(-time.Minute), fixture.clock.Now().Add(time.Minute), domain.Page{Limit: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != usage.ID {
		t.Fatalf("usage list total=%d items=%+v err=%v", total, items, err)
	}
	stopping, err := fixture.service.StopDeployment(fixture.opCtx, deployment.ID, deployment.Version, reservation.Version)
	if err != nil || stopping.Status != domain.DeploymentStopping {
		t.Fatalf("start stop=%+v err=%v", stopping, err)
	}
	stopped, err := fixture.service.StopDeployment(fixture.opCtx, deployment.ID, stopping.Version, reservation.Version)
	if err != nil || stopped.Status != domain.DeploymentStopped || stopped.StoppedAt == nil {
		t.Fatalf("complete stop=%+v err=%v", stopped, err)
	}
	persistedReservation, err := fixture.store.FindReservation(context.Background(), reservation.ID)
	if err != nil || persistedReservation.Status != domain.ReservationReleased {
		t.Fatalf("released reservation=%+v err=%v", persistedReservation, err)
	}
	persistedSite, _ = fixture.store.FindSite(context.Background(), site.ID)
	if persistedSite.AvailableUnits != site.TotalUnits {
		t.Fatalf("capacity not returned: %+v", persistedSite)
	}
	logs, count, err := fixture.store.ListAudit(context.Background(), "deployment", deployment.ID, domain.Page{Limit: 20})
	if err != nil || count < 4 || len(logs) < 4 {
		t.Fatalf("deployment audit count=%d logs=%+v err=%v", count, logs, err)
	}
}

func TestConcurrentServiceReservationsPreserveCapacity(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 5)
	scenario := fixture.approvedScenario(t, 4)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := fixture.service.ReserveCapacity(fixture.partCtx, scenario.ID, site.ID, time.Hour)
			results <- err
		}()
	}
	close(start)
	var success, capacity int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrCapacity) {
			capacity++
		} else {
			t.Fatalf("unexpected reserve result: %v", err)
		}
	}
	if success != 1 || capacity != 1 {
		t.Fatalf("success=%d capacity=%d", success, capacity)
	}
	persisted, _ := fixture.store.FindSite(context.Background(), site.ID)
	if persisted.AvailableUnits != 1 {
		t.Fatalf("capacity invariant=%+v", persisted)
	}
}

func TestStaleVersionsLeaveStateUnchanged(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 6)
	if _, err := fixture.service.TransitionSite(fixture.opCtx, site.ID, domain.SiteSuspended, site.Version-1); !errors.Is(err, domain.ErrVersion) {
		t.Fatalf("stale site version error=%v", err)
	}
	persisted, _ := fixture.store.FindSite(context.Background(), site.ID)
	if persisted.Status != domain.SiteActive || persisted.Version != site.Version {
		t.Fatalf("stale transition changed site: %+v", persisted)
	}
	scenario := fixture.approvedScenario(t, 2)
	if _, err := fixture.service.DecideScenario(fixture.opCtx, scenario.ID, "rejected", "late decision", scenario.Version-1); !errors.Is(err, domain.ErrVersion) {
		t.Fatalf("stale scenario version error=%v", err)
	}
	persistedScenario, _ := fixture.store.FindScenario(context.Background(), scenario.ID)
	if persistedScenario.Status != domain.ScenarioApproved {
		t.Fatalf("stale decision changed scenario: %+v", persistedScenario)
	}
}

func TestExpiredReservationWorkerReturnsCapacity(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 8)
	scenario := fixture.approvedScenario(t, 3)
	reservation, err := fixture.service.ReserveCapacity(fixture.partCtx, scenario.ID, site.ID, time.Minute)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := fixture.service.ExpireReservation(context.Background(), reservation.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("early expiry error=%v", err)
	}
	fixture.clock.Advance(time.Minute)
	if err := fixture.service.ExpireReservation(context.Background(), reservation.ID); err != nil {
		t.Fatalf("expire at boundary: %v", err)
	}
	if err := fixture.service.ExpireReservation(context.Background(), reservation.ID); err != nil {
		t.Fatalf("expiry replay should be idempotent: %v", err)
	}
	persisted, _ := fixture.store.FindSite(context.Background(), site.ID)
	if persisted.AvailableUnits != site.TotalUnits {
		t.Fatalf("capacity not restored: %+v", persisted)
	}
}

func TestContextCancellationStopsBulkReconciliation(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx, cancel := context.WithCancel(fixture.opCtx)
	cancel()
	results, err := fixture.service.ReconcileExpiredReservations(ctx, []string{"one", "two"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got results=%+v err=%v", results, err)
	}
	if len(results) != 0 {
		t.Fatalf("cancelled operation processed items: %+v", results)
	}
}
