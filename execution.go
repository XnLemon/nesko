package nesko

import (
	"errors"
	"time"
)

var (
	ErrExecutionRunNotFound = errors.New("execution run not found")
	ErrExecutionRunConflict = errors.New("execution run state conflict")
	ErrExecutionRunNotStale = errors.New("execution run is not stale")
)

// ExecutionInput identifies the ticket and run passed to an executor.
type ExecutionInput struct {
	Ticket    Ticket
	RunID     string
	Workspace Workspace
}

// Workspace identifies the caller-managed directory for one execution.
type Workspace struct {
	ID        string
	Directory string
}

// ExecutionRunStatus describes the durable state of one execution attempt.
type ExecutionRunStatus string

const (
	ExecutionRunRunning      ExecutionRunStatus = "running"
	ExecutionRunCompleted    ExecutionRunStatus = "completed"
	ExecutionRunNeedsReview  ExecutionRunStatus = "needs_review"
	ExecutionRunWaitingHuman ExecutionRunStatus = "waiting_human"
	ExecutionRunFailed       ExecutionRunStatus = "failed"
	ExecutionRunInterrupted  ExecutionRunStatus = "interrupted"
)

// ExecutionRun records one immutable instruction snapshot and its outcome.
type ExecutionRun struct {
	ID          string
	TicketID    string
	Instruction string
	Decision    *TicketDecision
	Status      ExecutionRunStatus
	StartedAt   time.Time
	FinishedAt  *time.Time
	// LastHeartbeatAt is nil until the run has persisted a heartbeat event.
	LastHeartbeatAt *time.Time
	Outcome         ExecutionOutcome
	Summary         string
	FailureReason   string
}

// ExecutionRunFinish describes a terminal update to an execution run.
type ExecutionRunFinish struct {
	Status        ExecutionRunStatus
	At            time.Time
	Result        ExecutionResult
	FailureReason string
}

// ExecutionEventType identifies a lifecycle, heartbeat, or progress event.
type ExecutionEventType string

const (
	ExecutionEventRunStarted       ExecutionEventType = "run_started"
	ExecutionEventWorkspaceReady   ExecutionEventType = "workspace_ready"
	ExecutionEventWorkspaceRelease ExecutionEventType = "workspace_released"
	ExecutionEventHeartbeat        ExecutionEventType = "heartbeat"
	ExecutionEventProgress         ExecutionEventType = "progress"
	ExecutionEventCompleted        ExecutionEventType = "completed"
	ExecutionEventNeedsReview      ExecutionEventType = "needs_review"
	ExecutionEventWaitingHuman     ExecutionEventType = "waiting_human"
	ExecutionEventFailed           ExecutionEventType = "failed"
	ExecutionEventInterrupted      ExecutionEventType = "interrupted"
)

// ExecutionEvent carries a lifecycle, heartbeat, or progress update.
type ExecutionEvent struct {
	ID     string
	Type   ExecutionEventType
	At     time.Time
	Detail string
}

// ExecutionRunEvent adds the durable ordering assigned to a run event.
type ExecutionRunEvent struct {
	Sequence int64
	RunID    string
	Event    ExecutionEvent
}

// ExecutionOutcome is the terminal handling result returned by an executor.
type ExecutionOutcome string

const (
	ExecutionOutcomeCompleted    ExecutionOutcome = "completed"
	ExecutionOutcomeNeedsReview  ExecutionOutcome = "needs_review"
	ExecutionOutcomeWaitingHuman ExecutionOutcome = "waiting_human"
)

// ExecutionResult summarizes a completed executor run.
type ExecutionResult struct {
	Outcome ExecutionOutcome
	Summary string
}
