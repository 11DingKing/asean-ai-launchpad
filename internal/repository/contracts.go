package repository

import (
	"context"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

type Identity interface {
	CreateUser(context.Context, domain.User) error
	FindUserByEmail(context.Context, string) (domain.User, error)
	FindUserByID(context.Context, string) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	FindSessionByTokenHash(context.Context, string) (domain.Session, domain.User, error)
	TouchSession(context.Context, string, time.Time) error
	RevokeSession(context.Context, string, time.Time) error
	DeleteExpiredSessions(context.Context, time.Time) (int64, error)
}

type Sites interface {
	CreateSite(context.Context, domain.ComputeSite) error
	FindSite(context.Context, string) (domain.ComputeSite, error)
	ListSites(context.Context, string, domain.SiteStatus, domain.Page) ([]domain.ComputeSite, int, error)
	ReviewSite(context.Context, domain.ComputeSite, domain.ComplianceReview, domain.AuditEvent) error
	TransitionSite(context.Context, domain.ComputeSite, domain.AuditEvent) error
}

type Scenarios interface {
	CreateScenario(context.Context, domain.Scenario) error
	FindScenario(context.Context, string) (domain.Scenario, error)
	ListScenarios(context.Context, domain.ScenarioStatus, domain.Page) ([]domain.Scenario, int, error)
	StartScenarioReview(context.Context, domain.Scenario, domain.AuditEvent) error
	DecideScenario(context.Context, domain.Scenario, domain.ScenarioReview, domain.AuditEvent) error
}

type Reservations interface {
	CreateReservation(context.Context, domain.Reservation, domain.Job, domain.AuditEvent) error
	FindReservation(context.Context, string) (domain.Reservation, error)
	ReleaseReservation(context.Context, domain.Reservation, domain.AuditEvent) error
	ExpireReservation(context.Context, domain.Reservation, domain.AuditEvent) error
}

type Deployments interface {
	CreateDeployment(context.Context, domain.Deployment, domain.Job, domain.AuditEvent) error
	FindDeployment(context.Context, string) (domain.Deployment, error)
	FindDeploymentByReservation(context.Context, string) (domain.Deployment, error)
	TransitionDeployment(context.Context, domain.Deployment, domain.AuditEvent) error
	StopDeploymentAndRelease(context.Context, domain.Deployment, domain.Reservation, domain.AuditEvent) error
}

type Metering interface {
	RecordUsageAndSettlement(context.Context, domain.UsageRecord, domain.SettlementEntry, domain.AuditEvent) error
	ListUsage(context.Context, string, time.Time, time.Time, domain.Page) ([]domain.UsageRecord, int, error)
}

type Jobs interface {
	ClaimJob(context.Context, string, time.Time, time.Duration) (domain.Job, error)
	CompleteJob(context.Context, string, string, time.Time) error
	RetryJob(context.Context, domain.Job, string, time.Time) error
	CancelJob(context.Context, string, time.Time) error
}

type Audit interface {
	ListAudit(context.Context, string, string, domain.Page) ([]domain.AuditEvent, int, error)
}

type Idempotency interface {
	ClaimIdempotency(context.Context, domain.IdempotencyRecord) (domain.IdempotencyRecord, bool, error)
	CompleteIdempotency(context.Context, string, int, string) error
	AbandonIdempotency(context.Context, string) error
}

type Readiness interface {
	Ping(context.Context) error
}

type Store interface {
	Identity
	Sites
	Scenarios
	Reservations
	Deployments
	Metering
	Jobs
	Audit
	Idempotency
	Readiness
}
