package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "launchpad.db")
	item, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = item.Close() })
	return item, path
}

func testTime() time.Time {
	return time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
}

func insertUser(t *testing.T, store *Store, id string, role domain.Role) domain.User {
	t.Helper()
	now := testTime()
	item := domain.User{ID: id, Email: id + "@example.test", PasswordHash: "hash", Role: role, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(context.Background(), item); err != nil {
		t.Fatalf("create user %s: %v", id, err)
	}
	return item
}

func insertActiveSite(t *testing.T, store *Store, operator domain.User, units int) domain.ComputeSite {
	t.Helper()
	item, err := domain.NewComputeSite("site_1", operator.ID, "Singapore Regional GPU", "SG", "Asia/Singapore", units, testTime())
	if err != nil {
		t.Fatalf("new site: %v", err)
	}
	item.Status = domain.SiteActive
	if err := store.CreateSite(context.Background(), item); err != nil {
		t.Fatalf("create site: %v", err)
	}
	return item
}

func insertApprovedScenario(t *testing.T, store *Store, partner domain.User, units int) domain.Scenario {
	t.Helper()
	item, err := domain.NewScenario("scenario_1", partner.ID, "Maritime language intelligence", "maritime", "restricted", units, testTime())
	if err != nil {
		t.Fatalf("new scenario: %v", err)
	}
	item.Status = domain.ScenarioApproved
	if err := store.CreateScenario(context.Background(), item); err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	return item
}

func auditEvent(id, actor, action, objectType, objectID string) domain.AuditEvent {
	return domain.AuditEvent{ID: id, ActorID: actor, Action: action, ObjectType: objectType, ObjectID: objectID, Result: "success", RequestID: "req_test", Details: `{}`, CreatedAt: testTime()}
}

func pendingJob(id, kind, aggregate string, at time.Time) domain.Job {
	return domain.Job{ID: id, Kind: kind, AggregateID: aggregate, Payload: `{}`, Status: domain.JobPending, MaxAttempts: 3, AvailableAt: at, CreatedAt: at, UpdatedAt: at}
}

func TestOpenAppliesMigrationsAndCanRestart(t *testing.T) {
	store, path := openTestStore(t)
	ctx := context.Background()
	var migrationCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 2 {
		t.Fatalf("migration count=%d, want 2", migrationCount)
	}
	tables := []string{"users", "sessions", "compute_sites", "compliance_reviews", "scenarios", "scenario_reviews", "reservations", "deployments", "usage_records", "settlement_entries", "jobs", "audit_events", "idempotency_records"}
	for _, table := range tables {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatalf("find table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s count=%d", table, count)
		}
	}
	user := insertUser(t, store, "operator_restart", domain.RoleOperator)
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	got, err := reopened.FindUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find persisted user: %v", err)
	}
	if got.Email != user.Email || got.Role != user.Role {
		t.Fatalf("persisted user mismatch: %+v", got)
	}
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations after restart: %v", err)
	}
	if migrationCount != 2 {
		t.Fatalf("restart reapplied migrations: %d", migrationCount)
	}
}

func TestUnknownMigrationHistoryBlocksStartup(t *testing.T) {
	store, path := openTestStore(t)
	if _, err := store.db.Exec(`INSERT INTO schema_migrations(version,name,applied_at) VALUES(99,'unknown.sql',?)`, timestamp(testTime())); err != nil {
		t.Fatalf("insert unknown migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("expected startup failure for unknown history")
	}
}

func TestIdentitySessionLifecycle(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	user := insertUser(t, store, "partner_session", domain.RolePartner)
	now := testTime()
	session := domain.Session{ID: "session_1", UserID: user.ID, TokenHash: "token_hash_1", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastSeenAt: now}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	gotSession, gotUser, err := store.FindSessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if gotSession.ID != session.ID || gotUser.ID != user.ID {
		t.Fatalf("session join mismatch: %+v %+v", gotSession, gotUser)
	}
	touched := now.Add(10 * time.Minute)
	if err := store.TouchSession(ctx, session.ID, touched); err != nil {
		t.Fatalf("touch session: %v", err)
	}
	gotSession, _, _ = store.FindSessionByTokenHash(ctx, session.TokenHash)
	if !gotSession.LastSeenAt.Equal(touched) {
		t.Fatalf("last seen=%s", gotSession.LastSeenAt)
	}
	if err := store.RevokeSession(ctx, session.ID, touched); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	gotSession, _, _ = store.FindSessionByTokenHash(ctx, session.TokenHash)
	if gotSession.RevokedAt == nil || !gotSession.RevokedAt.Equal(touched) {
		t.Fatalf("revocation missing: %+v", gotSession)
	}
	if err := store.TouchSession(ctx, session.ID, touched.Add(time.Minute)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked touch should fail: %v", err)
	}
	if err := store.RevokeSession(ctx, session.ID, touched.Add(time.Minute)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("second revocation should fail: %v", err)
	}
}

func TestExpiredSessionCleanupPreservesRecentRevocation(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	user := insertUser(t, store, "partner_cleanup", domain.RolePartner)
	now := testTime()
	sessions := []domain.Session{
		{ID: "expired", UserID: user.ID, TokenHash: "expired_hash", ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), LastSeenAt: now.Add(-time.Hour)},
		{ID: "active", UserID: user.ID, TokenHash: "active_hash", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastSeenAt: now},
		{ID: "revoked", UserID: user.ID, TokenHash: "revoked_hash", ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(-48 * time.Hour), LastSeenAt: now.Add(-48 * time.Hour)},
	}
	for _, session := range sessions {
		if err := store.CreateSession(ctx, session); err != nil {
			t.Fatalf("create %s: %v", session.ID, err)
		}
	}
	if err := store.RevokeSession(ctx, "revoked", now.Add(-25*time.Hour)); err != nil {
		t.Fatalf("backdate revocation: %v", err)
	}
	count, err := store.DeleteExpiredSessions(ctx, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if count != 2 {
		t.Fatalf("deleted=%d want=2", count)
	}
	if _, _, err := store.FindSessionByTokenHash(ctx, "active_hash"); err != nil {
		t.Fatalf("active session removed: %v", err)
	}
}

func TestUserAndResourceUniquenessAndForeignKeys(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_unique", domain.RoleOperator)
	if err := store.CreateUser(ctx, operator); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate user error=%v", err)
	}
	site := insertActiveSite(t, store, operator, 10)
	duplicate := site
	duplicate.ID = "site_duplicate"
	if err := store.CreateSite(ctx, duplicate); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate site identity error=%v", err)
	}
	orphan := site
	orphan.ID = "site_orphan"
	orphan.Name = "Orphan Site"
	orphan.OperatorID = "missing_user"
	if err := store.CreateSite(ctx, orphan); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("foreign key error=%v", err)
	}
}

func TestSiteReviewTransactionRollsBackWhenAuditFails(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_review", domain.RoleOperator)
	site, err := domain.NewComputeSite("site_review", operator.ID, "Reviewable Compute Site", "MY", "Asia/Kuala_Lumpur", 20, testTime())
	if err != nil {
		t.Fatalf("new site: %v", err)
	}
	if err := store.CreateSite(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := site.Transition(domain.SiteVerified, testTime().Add(time.Minute)); err != nil {
		t.Fatalf("transition domain: %v", err)
	}
	review := domain.ComplianceReview{ID: "review_1", SiteID: site.ID, ReviewerID: operator.ID, Decision: "approved", EvidenceRef: "evidence://review", CreatedAt: testTime()}
	badAudit := auditEvent("audit_duplicate", operator.ID, "site.review", "compute_site", site.ID)
	if err := insertAudit(ctx, store.db, badAudit); err != nil {
		t.Fatalf("seed duplicate audit: %v", err)
	}
	if err := store.ReviewSite(ctx, site, review, badAudit); err == nil {
		t.Fatal("expected audit insertion failure")
	}
	persisted, err := store.FindSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("find rolled back site: %v", err)
	}
	if persisted.Status != domain.SiteDraft || persisted.Version != 1 {
		t.Fatalf("site update leaked across rollback: %+v", persisted)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM compliance_reviews WHERE site_id=?`, site.ID).Scan(&count); err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if count != 0 {
		t.Fatalf("review leaked across rollback: %d", count)
	}
}

func TestReservationTransactionClaimsCapacityAndWritesJobAudit(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_reserve", domain.RoleOperator)
	partner := insertUser(t, store, "partner_reserve", domain.RolePartner)
	site := insertActiveSite(t, store, operator, 10)
	scenario := insertApprovedScenario(t, store, partner, 4)
	reservation := domain.Reservation{ID: "reservation_1", ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partner.ID, Units: 4, Status: domain.ReservationConfirmed, ExpiresAt: testTime().Add(time.Hour), Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
	job := pendingJob("job_expire", "expire_reservation", reservation.ID, reservation.ExpiresAt)
	audit := auditEvent("audit_reserve", partner.ID, "reservation.create", "reservation", reservation.ID)
	if err := store.CreateReservation(ctx, reservation, job, audit); err != nil {
		t.Fatalf("create reservation transaction: %v", err)
	}
	persistedSite, err := store.FindSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("find site: %v", err)
	}
	if persistedSite.AvailableUnits != 6 || persistedSite.Version != 2 {
		t.Fatalf("capacity claim mismatch: %+v", persistedSite)
	}
	persistedReservation, err := store.FindReservation(ctx, reservation.ID)
	if err != nil || persistedReservation.Units != 4 {
		t.Fatalf("reservation mismatch: %+v %v", persistedReservation, err)
	}
	var jobCount, auditCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE aggregate_id=?`, reservation.ID).Scan(&jobCount)
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE object_id=?`, reservation.ID).Scan(&auditCount)
	if jobCount != 1 || auditCount != 1 {
		t.Fatalf("transaction records job=%d audit=%d", jobCount, auditCount)
	}
}

func TestReservationTransactionRollsBackCapacityOnDuplicateJob(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_rollback", domain.RoleOperator)
	partner := insertUser(t, store, "partner_rollback", domain.RolePartner)
	site := insertActiveSite(t, store, operator, 10)
	scenario := insertApprovedScenario(t, store, partner, 4)
	job := pendingJob("existing_job", "expire_reservation", "reservation_rollback", testTime())
	if err := insertJob(ctx, store.db, job); err != nil {
		t.Fatalf("insert existing job: %v", err)
	}
	reservation := domain.Reservation{ID: "reservation_rollback", ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partner.ID, Units: 4, Status: domain.ReservationConfirmed, ExpiresAt: testTime().Add(time.Hour), Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
	duplicate := pendingJob("different_id", "expire_reservation", reservation.ID, reservation.ExpiresAt)
	err := store.CreateReservation(ctx, reservation, duplicate, auditEvent("audit_rollback", partner.ID, "reservation.create", "reservation", reservation.ID))
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected duplicate job error, got %v", err)
	}
	persistedSite, _ := store.FindSite(ctx, site.ID)
	if persistedSite.AvailableUnits != 10 || persistedSite.Version != 1 {
		t.Fatalf("capacity leaked on rollback: %+v", persistedSite)
	}
	if _, err := store.FindReservation(ctx, reservation.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reservation leaked on rollback: %v", err)
	}
}

func TestConcurrentReservationsDoNotOversell(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_concurrent", domain.RoleOperator)
	partner := insertUser(t, store, "partner_concurrent", domain.RolePartner)
	site := insertActiveSite(t, store, operator, 5)
	scenario := insertApprovedScenario(t, store, partner, 4)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			id := fmt.Sprintf("reservation_concurrent_%d", index)
			item := domain.Reservation{ID: id, ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partner.ID, Units: 4, Status: domain.ReservationConfirmed, ExpiresAt: testTime().Add(time.Hour), Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
			job := pendingJob(fmt.Sprintf("job_concurrent_%d", index), "expire_reservation", id, item.ExpiresAt)
			results <- store.CreateReservation(ctx, item, job, auditEvent(fmt.Sprintf("audit_concurrent_%d", index), partner.ID, "reservation.create", "reservation", id))
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, capacityFailures int
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrCapacity) {
			capacityFailures++
		} else {
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || capacityFailures != 1 {
		t.Fatalf("successes=%d capacityFailures=%d", successes, capacityFailures)
	}
	persisted, _ := store.FindSite(ctx, site.ID)
	if persisted.AvailableUnits != 1 {
		t.Fatalf("oversell invariant broken: %+v", persisted)
	}
}

func TestReleaseReservationReturnsCapacityExactlyOnce(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_release", domain.RoleOperator)
	partner := insertUser(t, store, "partner_release", domain.RolePartner)
	site := insertActiveSite(t, store, operator, 9)
	scenario := insertApprovedScenario(t, store, partner, 3)
	item := domain.Reservation{ID: "reservation_release", ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partner.ID, Units: 3, Status: domain.ReservationConfirmed, ExpiresAt: testTime().Add(time.Hour), Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
	if err := store.CreateReservation(ctx, item, pendingJob("job_release", "expire_reservation", item.ID, item.ExpiresAt), auditEvent("audit_create_release", partner.ID, "reservation.create", "reservation", item.ID)); err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	if err := item.Transition(domain.ReservationReleased, testTime().Add(time.Minute)); err != nil {
		t.Fatalf("transition reservation: %v", err)
	}
	if err := store.ReleaseReservation(ctx, item, auditEvent("audit_release", partner.ID, "reservation.release", "reservation", item.ID)); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := store.ReleaseReservation(ctx, item, auditEvent("audit_release_again", partner.ID, "reservation.release", "reservation", item.ID)); !errors.Is(err, domain.ErrVersion) {
		t.Fatalf("second release should conflict: %v", err)
	}
	persisted, _ := store.FindSite(ctx, site.ID)
	if persisted.AvailableUnits != 9 {
		t.Fatalf("capacity returned more than once: %+v", persisted)
	}
}

func TestJobLeaseRetryAndPermanentFailure(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	now := testTime()
	job := pendingJob("job_retry", "activate_deployment", "dep_1", now)
	job.MaxAttempts = 2
	if err := insertJob(ctx, store.db, job); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	claimed, err := store.ClaimJob(ctx, "worker_a", now, time.Minute)
	if err != nil {
		t.Fatalf("claim first: %v", err)
	}
	if claimed.Status != domain.JobRunning || claimed.Attempts != 1 || claimed.LeaseOwner != "worker_a" {
		t.Fatalf("first claim mismatch: %+v", claimed)
	}
	if _, err := store.ClaimJob(ctx, "worker_b", now.Add(30*time.Second), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("active lease should block claim: %v", err)
	}
	if err := store.RetryJob(ctx, claimed, "temporary", now); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	if _, err := store.ClaimJob(ctx, "worker_b", now, time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("backoff should delay claim: %v", err)
	}
	claimed, err = store.ClaimJob(ctx, "worker_b", now.Add(time.Second), time.Minute)
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	if claimed.Attempts != 2 {
		t.Fatalf("attempts=%d", claimed.Attempts)
	}
	if err := store.RetryJob(ctx, claimed, "permanent", now.Add(time.Second)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	var status string
	var lastError string
	if err := store.db.QueryRow(`SELECT status,last_error FROM jobs WHERE id=?`, job.ID).Scan(&status, &lastError); err != nil {
		t.Fatalf("read failed job: %v", err)
	}
	if status != "failed" || lastError != "permanent" {
		t.Fatalf("failed job status=%s error=%q", status, lastError)
	}
}

func TestExpiredLeaseCanBeRecoveredAfterRestart(t *testing.T) {
	store, path := openTestStore(t)
	ctx := context.Background()
	now := testTime()
	job := pendingJob("job_recover", "cleanup_sessions", "global", now)
	if err := insertJob(ctx, store.db, job); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	first, err := store.ClaimJob(ctx, "worker_dead", now, time.Second)
	if err != nil {
		t.Fatalf("claim dead worker: %v", err)
	}
	if first.LeaseOwner != "worker_dead" {
		t.Fatalf("lease owner=%s", first.LeaseOwner)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	if _, err := reopened.ClaimJob(ctx, "worker_new", now.Add(500*time.Millisecond), time.Second); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unexpired lease should survive restart: %v", err)
	}
	recovered, err := reopened.ClaimJob(ctx, "worker_new", now.Add(2*time.Second), time.Second)
	if err != nil {
		t.Fatalf("recover expired lease: %v", err)
	}
	if recovered.LeaseOwner != "worker_new" || recovered.Attempts != 2 {
		t.Fatalf("recovered job mismatch: %+v", recovered)
	}
}

func TestUsageAndSettlementRollbackTogether(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	operator := insertUser(t, store, "operator_meter", domain.RoleOperator)
	partner := insertUser(t, store, "partner_meter", domain.RolePartner)
	site := insertActiveSite(t, store, operator, 8)
	scenario := insertApprovedScenario(t, store, partner, 2)
	reservation := domain.Reservation{ID: "reservation_meter", ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partner.ID, Units: 2, Status: domain.ReservationConfirmed, ExpiresAt: testTime().Add(time.Hour), Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
	if err := store.CreateReservation(ctx, reservation, pendingJob("job_meter_expire", "expire_reservation", reservation.ID, reservation.ExpiresAt), auditEvent("audit_meter_reserve", partner.ID, "reservation.create", "reservation", reservation.ID)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	deployment := domain.Deployment{ID: "deployment_meter", ReservationID: reservation.ID, Status: domain.DeploymentRunning, Endpoint: "https://runtime", Version: 1, CreatedAt: testTime(), UpdatedAt: testTime()}
	if err := store.CreateDeployment(ctx, deployment, pendingJob("job_meter_activate", "activate_deployment", deployment.ID, testTime()), auditEvent("audit_meter_deploy", partner.ID, "deployment.create", "deployment", deployment.ID)); err != nil {
		t.Fatalf("deployment: %v", err)
	}
	usage := domain.UsageRecord{ID: "usage_1", DeploymentID: deployment.ID, PartnerID: partner.ID, PeriodStart: testTime(), PeriodEnd: testTime().Add(time.Minute), UnitSeconds: 120, CreatedAt: testTime().Add(time.Minute)}
	settlement := domain.SettlementEntry{ID: "settlement_1", UsageID: usage.ID, PartnerID: "missing_partner", AmountMicros: 2040, Currency: "USD", CreatedAt: usage.CreatedAt}
	err := store.RecordUsageAndSettlement(ctx, usage, settlement, auditEvent("audit_usage_bad", operator.ID, "usage.record", "deployment", deployment.ID))
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected invalid settlement owner, got %v", err)
	}
	var usageCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE id=?`, usage.ID).Scan(&usageCount); err != nil {
		t.Fatalf("count usage: %v", err)
	}
	if usageCount != 0 {
		t.Fatalf("usage leaked after settlement rollback: %d", usageCount)
	}
	settlement.PartnerID = partner.ID
	if err := store.RecordUsageAndSettlement(ctx, usage, settlement, auditEvent("audit_usage_good", operator.ID, "usage.record", "deployment", deployment.ID)); err != nil {
		t.Fatalf("record usage and settlement: %v", err)
	}
	items, total, err := store.ListUsage(ctx, partner.ID, testTime().Add(-time.Minute), testTime().Add(time.Hour), domain.Page{Limit: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != usage.ID {
		t.Fatalf("usage list mismatch total=%d items=%+v err=%v", total, items, err)
	}
}

func TestContextCancellationPropagatesToDatabase(t *testing.T) {
	store, _ := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.FindUserByID(ctx, "anything"); !errors.Is(err, context.Canceled) {
		t.Fatalf("database error should preserve cancellation, got %v", err)
	}
	if _, err := store.DeleteExpiredSessions(ctx, testTime()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup should preserve cancellation, got %v", err)
	}
}
