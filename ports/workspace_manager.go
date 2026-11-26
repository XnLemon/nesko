package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// WorkspaceManager prepares and releases caller-owned workspaces for runs.
// Prepare must clean up partial resources when it returns an error. Release is
// called after every successful Prepare, including when execution fails.
type WorkspaceManager interface {
	Prepare(ctx context.Context, input nesko.ExecutionInput) (nesko.Workspace, error)
	Release(ctx context.Context, input nesko.ExecutionInput, result nesko.ExecutionResult, executionErr error) error
}
