package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// ExecutionRunRepository stores run snapshots and their ordered event history.
type ExecutionRunRepository interface {
	StartExecutionRun(ctx context.Context, run nesko.ExecutionRun) error
	RecordExecutionEvent(ctx context.Context, runID string, event nesko.ExecutionEvent) error
	FinishExecutionRun(ctx context.Context, runID string, finish nesko.ExecutionRunFinish) error
	GetExecutionRun(ctx context.Context, runID string) (nesko.ExecutionRun, error)
	ListActiveExecutionRuns(ctx context.Context) ([]nesko.ExecutionRun, error)
	ListExecutionRunEvents(ctx context.Context, runID string) ([]nesko.ExecutionRunEvent, error)
}
