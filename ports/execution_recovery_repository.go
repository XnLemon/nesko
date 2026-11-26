package ports

import (
	"context"
	"time"
)

// ExecutionRecoveryRepository exposes recovery mutations for a caller-owned supervisor.
type ExecutionRecoveryRepository interface {
	// InterruptStaleExecution rechecks the heartbeat cutoff and atomically marks
	// the run interrupted and its matching running ticket failed. The caller
	// chooses the cutoff and decides what to do after interruption.
	InterruptStaleExecution(ctx context.Context, runID string, heartbeatBefore, at time.Time, reason string) error
}
