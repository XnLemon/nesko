package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// Judge assesses a ticket without selecting its handling route.
type Judge interface {
	Assess(ctx context.Context, ticket nesko.Ticket) (nesko.JudgeAssessment, error)
}
