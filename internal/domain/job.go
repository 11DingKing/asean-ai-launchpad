package domain

import "time"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

type Job struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	AggregateID string     `json:"aggregate_id"`
	Payload     string     `json:"payload"`
	Status      JobStatus  `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	AvailableAt time.Time  `json:"available_at"`
	LeaseOwner  string     `json:"lease_owner"`
	LeaseUntil  *time.Time `json:"lease_until,omitempty"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type AuditEvent struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id,omitempty"`
	Action     string    `json:"action"`
	ObjectType string    `json:"object_type"`
	ObjectID   string    `json:"object_id"`
	Result     string    `json:"result"`
	RequestID  string    `json:"request_id"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

type IdempotencyRecord struct {
	Scope          string
	ActorID        string
	Method         string
	Path           string
	RequestHash    string
	ResponseStatus int
	ResponseBody   string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

type Page struct {
	Limit  int
	Offset int
}

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
