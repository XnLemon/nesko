package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// ExecutionReporter persists progress emitted by an executor.
type ExecutionReporter func(nesko.ExecutionEvent) error

// Executor runs a judged ticket and reports heartbeat or progress events.
type Executor interface {
	Execute(ctx context.Context, input nesko.ExecutionInput, report ExecutionReporter) (nesko.ExecutionResult, error)
}

// ExecutorRouter selects the execution core for a ticket.
type ExecutorRouter interface {
	Route(ctx context.Context, input nesko.ExecutionInput) (Executor, error)
}

// ExecutorRouterFunc adapts a function into an ExecutorRouter.
type ExecutorRouterFunc func(context.Context, nesko.ExecutionInput) (Executor, error)

// Route selects an executor through the adapted function.
func (f ExecutorRouterFunc) Route(ctx context.Context, input nesko.ExecutionInput) (Executor, error) {
	return f(ctx, input)
}
