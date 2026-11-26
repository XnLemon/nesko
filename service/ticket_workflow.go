package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

// TicketWorkflow coordinates judging, review gates, execution, and heartbeats.
type TicketWorkflow struct {
	tickets    ports.TicketRepository
	judge      ports.Judge
	policy     DecisionPolicy
	router     ports.ExecutorRouter
	runs       ports.ExecutionRunRepository
	lifecycle  ports.ExecutionLifecycleRepository
	workspaces ports.WorkspaceManager
	logger     Logger
}

// TicketWorkflowDependencies are the components used by the ticket workflow.
type TicketWorkflowDependencies struct {
	Tickets    ports.TicketRepository
	Judge      ports.Judge
	Policy     DecisionPolicy
	Router     ports.ExecutorRouter
	Runs       ports.ExecutionRunRepository
	Lifecycle  ports.ExecutionLifecycleRepository
	Workspaces ports.WorkspaceManager
	Logger     Logger
}

// NewTicketWorkflow wires the workflow dependencies and defaults a missing policy.
func NewTicketWorkflow(dependencies TicketWorkflowDependencies) (*TicketWorkflow, error) {
	if dependencies.Tickets == nil {
		return nil, errors.New("ticket repository is required")
	}
	if dependencies.Judge == nil {
		return nil, errors.New("judge is required")
	}
	if dependencies.Router == nil {
		return nil, errors.New("executor router is required")
	}
	if dependencies.Lifecycle != nil && dependencies.Runs == nil {
		return nil, errors.New("execution run repository is required with an execution lifecycle repository")
	}
	if dependencies.Policy == nil {
		dependencies.Policy = DefaultDecisionPolicy{}
	}
	if dependencies.Logger == nil {
		dependencies.Logger = slog.Default()
	}
	return &TicketWorkflow{
		tickets:    dependencies.Tickets,
		judge:      dependencies.Judge,
		policy:     dependencies.Policy,
		router:     dependencies.Router,
		runs:       dependencies.Runs,
		lifecycle:  dependencies.Lifecycle,
		workspaces: dependencies.Workspaces,
		logger:     dependencies.Logger,
	}, nil
}

// Triage assesses a newly received or failed ticket and runs automatic decisions.
func (w *TicketWorkflow) Triage(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	ticket, err := w.Assess(ctx, ticketID)
	if err != nil {
		return nesko.Ticket{}, err
	}
	if ticket.Status == nesko.TicketStatusQueued {
		return w.Execute(ctx, ticketID)
	}
	return ticket, nil
}

// Assess persists a decision without starting the executor.
func (w *TicketWorkflow) Assess(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	ticket, err := w.tickets.Get(ctx, ticketID)
	if err != nil {
		return nesko.Ticket{}, err
	}
	if ticket.Status != nesko.TicketStatusReceived && ticket.Status != nesko.TicketStatusFailed {
		return nesko.Ticket{}, fmt.Errorf("%w: cannot triage ticket in %s", nesko.ErrTicketStatusConflict, ticket.Status)
	}
	if err := w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
		From:          ticket.Status,
		To:            nesko.TicketStatusTriaging,
		At:            time.Now().UTC(),
		ClearDecision: true,
		ClearFailure:  true,
	}); err != nil {
		return nesko.Ticket{}, err
	}
	return w.assess(ctx, ticketID)
}

// Resume adds human input to a waiting ticket and asks the Judge to reassess it.
func (w *TicketWorkflow) Resume(ctx context.Context, ticketID, humanInput string) (nesko.Ticket, error) {
	ticket, err := w.ResumeAndAssess(ctx, ticketID, humanInput)
	if err != nil {
		return nesko.Ticket{}, err
	}
	if ticket.Status == nesko.TicketStatusQueued {
		return w.Execute(ctx, ticketID)
	}
	return ticket, nil
}

// ResumeAndAssess adds human input and returns the new decision without execution.
func (w *TicketWorkflow) ResumeAndAssess(ctx context.Context, ticketID, humanInput string) (nesko.Ticket, error) {
	if _, err := w.tickets.ResumeWithHumanInput(ctx, ticketID, humanInput, time.Now().UTC()); err != nil {
		return nesko.Ticket{}, err
	}
	return w.assess(ctx, ticketID)
}

// ApprovePlan releases a Judge-gated ticket into the queued state.
func (w *TicketWorkflow) ApprovePlan(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	if err := w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
		From: nesko.TicketStatusAwaitingApproval,
		To:   nesko.TicketStatusQueued,
		At:   time.Now().UTC(),
	}); err != nil {
		return nesko.Ticket{}, err
	}
	return w.tickets.Get(ctx, ticketID)
}

// ApproveReview completes a ticket after its execution output is reviewed.
func (w *TicketWorkflow) ApproveReview(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	if err := w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
		From: nesko.TicketStatusNeedsReview,
		To:   nesko.TicketStatusCompleted,
		At:   time.Now().UTC(),
	}); err != nil {
		return nesko.Ticket{}, err
	}
	return w.tickets.Get(ctx, ticketID)
}

// RejectReview closes a ticket declined at either review gate.
func (w *TicketWorkflow) RejectReview(ctx context.Context, ticketID string) error {
	ticket, err := w.tickets.Get(ctx, ticketID)
	if err != nil {
		return err
	}
	if ticket.Status != nesko.TicketStatusAwaitingApproval && ticket.Status != nesko.TicketStatusNeedsReview {
		return fmt.Errorf("%w: ticket %s is not awaiting review", nesko.ErrTicketStatusConflict, ticketID)
	}
	return w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
		From: ticket.Status,
		To:   nesko.TicketStatusCancelled,
		At:   time.Now().UTC(),
	})
}

// Execute runs a queued ticket and persists executor heartbeat and progress events.
func (w *TicketWorkflow) Execute(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	input, err := w.startExecution(ctx, ticketID)
	if err != nil {
		return nesko.Ticket{}, err
	}
	input, workspaceAcquired, err := w.prepareWorkspace(ctx, input)
	if err != nil {
		return nesko.Ticket{}, w.failExecution(ctx, input, err)
	}
	result, err := w.runExecution(ctx, input)
	if workspaceAcquired {
		if releaseErr := w.finishWorkspace(ctx, input, result, err); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}
	if err != nil {
		return nesko.Ticket{}, w.failExecution(ctx, input, err)
	}
	return w.finishExecution(ctx, input, result)
}

func (w *TicketWorkflow) prepareWorkspace(ctx context.Context, input nesko.ExecutionInput) (nesko.ExecutionInput, bool, error) {
	if w.workspaces == nil {
		return input, false, nil
	}
	workspace, err := w.workspaces.Prepare(ctx, input)
	if err != nil {
		return input, false, fmt.Errorf("prepare execution workspace: %w", err)
	}
	input.Workspace = workspace
	if err := w.recordWorkspaceReady(ctx, input); err != nil {
		return input, false, w.releaseAfterPreparationFailure(ctx, input, err)
	}
	return input, true, nil
}

func (w *TicketWorkflow) recordWorkspaceReady(ctx context.Context, input nesko.ExecutionInput) error {
	if strings.TrimSpace(input.Workspace.Directory) == "" {
		return errors.New("workspace manager returned an empty directory")
	}
	if w.runs == nil {
		return nil
	}
	detail := input.Workspace.ID
	if detail == "" {
		detail = "workspace prepared"
	}
	return w.recordRunEvent(ctx, input.RunID, nesko.ExecutionEvent{
		Type:   nesko.ExecutionEventWorkspaceReady,
		At:     time.Now().UTC(),
		Detail: detail,
	})
}

func (w *TicketWorkflow) releaseAfterPreparationFailure(ctx context.Context, input nesko.ExecutionInput, cause error) error {
	if err := w.releaseWorkspace(ctx, input, nesko.ExecutionResult{}, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("release execution workspace: %w", err))
	}
	return cause
}

func (w *TicketWorkflow) finishWorkspace(ctx context.Context, input nesko.ExecutionInput, result nesko.ExecutionResult, executionErr error) error {
	releaseErr := w.releaseWorkspace(ctx, input, result, executionErr)
	combinedErr := executionErr
	if releaseErr != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("release execution workspace: %w", releaseErr))
	}
	if w.runs == nil {
		return combinedErr
	}
	eventCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	detail := "workspace released"
	if releaseErr != nil {
		detail = "workspace release failed"
	}
	if input.Workspace.ID != "" {
		detail += ": " + input.Workspace.ID
	}
	if err := w.recordRunEvent(eventCtx, input.RunID, nesko.ExecutionEvent{
		Type:   nesko.ExecutionEventWorkspaceRelease,
		At:     time.Now().UTC(),
		Detail: detail,
	}); err != nil {
		return errors.Join(combinedErr, fmt.Errorf("record workspace release: %w", err))
	}
	return combinedErr
}

func (w *TicketWorkflow) startExecution(ctx context.Context, ticketID string) (nesko.ExecutionInput, error) {
	ticket, err := w.tickets.Get(ctx, ticketID)
	if err != nil {
		return nesko.ExecutionInput{}, err
	}
	if ticket.Status != nesko.TicketStatusQueued {
		return nesko.ExecutionInput{}, fmt.Errorf("%w: ticket %s is not ready to execute", nesko.ErrTicketStatusConflict, ticketID)
	}
	if ticket.Decision == nil {
		err := errors.New("queued ticket has no judge decision")
		return nesko.ExecutionInput{}, w.fail(ctx, ticketID, nesko.TicketStatusQueued, err)
	}
	if err := ticket.Decision.Validate(); err != nil {
		return nesko.ExecutionInput{}, w.fail(ctx, ticketID, nesko.TicketStatusQueued, fmt.Errorf("ticket %s has an invalid decision: %w", ticketID, err))
	}

	runID := uuid.NewString()
	startedAt := time.Now().UTC()
	if w.lifecycle != nil {
		ticket, err = w.lifecycle.StartExecution(ctx, ticketID, runID, startedAt)
		if err != nil {
			return nesko.ExecutionInput{}, fmt.Errorf("start ticket execution: %w", err)
		}
	} else {
		if err := w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
			From:        nesko.TicketStatusQueued,
			To:          nesko.TicketStatusRunning,
			At:          startedAt,
			ExecutionID: runID,
		}); err != nil {
			return nesko.ExecutionInput{}, err
		}
		ticket, err = w.tickets.Get(ctx, ticketID)
		if err != nil {
			return nesko.ExecutionInput{}, w.fail(ctx, ticketID, nesko.TicketStatusRunning, err)
		}
		if w.runs != nil {
			if err := w.runs.StartExecutionRun(ctx, nesko.ExecutionRun{
				ID:          runID,
				TicketID:    ticket.ID,
				Instruction: ticket.RequestText,
				Decision:    ticket.Decision,
				Status:      nesko.ExecutionRunRunning,
				StartedAt:   startedAt,
			}); err != nil {
				cause := fmt.Errorf("start execution run record: %w", err)
				return nesko.ExecutionInput{}, w.failExecution(ctx, inputForRun(ticket, runID), cause)
			}
		}
	}
	return inputForRun(ticket, runID), nil
}

func inputForRun(ticket nesko.Ticket, runID string) nesko.ExecutionInput {
	return nesko.ExecutionInput{Ticket: ticket, RunID: runID}
}

func (w *TicketWorkflow) runExecution(ctx context.Context, input nesko.ExecutionInput) (nesko.ExecutionResult, error) {
	executor, err := w.router.Route(ctx, input)
	if err != nil {
		return nesko.ExecutionResult{}, fmt.Errorf("route execution: %w", err)
	}
	if executor == nil {
		return nesko.ExecutionResult{}, errors.New("executor router returned nil")
	}
	return executor.Execute(ctx, input, func(event nesko.ExecutionEvent) error {
		return w.reportExecutionEvent(ctx, input.Ticket.ID, input.RunID, event)
	})
}

func (w *TicketWorkflow) reportExecutionEvent(ctx context.Context, ticketID, runID string, event nesko.ExecutionEvent) error {
	if event.Type != nesko.ExecutionEventHeartbeat && event.Type != nesko.ExecutionEventProgress {
		return fmt.Errorf("unsupported execution event type %q", event.Type)
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	if err := w.recordRunEvent(ctx, runID, event); err != nil {
		return err
	}
	return w.tickets.RecordHeartbeat(ctx, ticketID, runID, event)
}

func (w *TicketWorkflow) finishExecution(ctx context.Context, input nesko.ExecutionInput, result nesko.ExecutionResult) (nesko.Ticket, error) {
	status, runStatus, err := statusesForOutcome(result.Outcome)
	if err != nil {
		return nesko.Ticket{}, w.failExecution(ctx, input, err)
	}
	if w.lifecycle != nil {
		finish := nesko.ExecutionRunFinish{Status: runStatus, At: time.Now().UTC(), Result: result}
		if err := w.lifecycle.CompleteExecution(ctx, input.Ticket.ID, input.RunID, status, finish); err != nil {
			cause := fmt.Errorf("persist execution result: %w", err)
			return nesko.Ticket{}, w.failExecution(ctx, input, cause)
		}
		w.logger.Info("ticket execution finished", "ticket", nesko.FormatTicketReference(input.Ticket), "run_id", input.RunID, "status", status)
		return w.tickets.Get(ctx, input.Ticket.ID)
	}
	if err := w.tickets.Transition(ctx, input.Ticket.ID, nesko.TicketTransition{
		From:             nesko.TicketStatusRunning,
		To:               status,
		At:               time.Now().UTC(),
		ExecutionSummary: result.Summary,
	}); err != nil {
		return nesko.Ticket{}, w.failExecution(ctx, input, err)
	}
	if w.runs != nil {
		if err := w.finishExecutionRun(ctx, input.RunID, nesko.ExecutionRunFinish{
			Status: runStatus,
			At:     time.Now().UTC(),
			Result: result,
		}); err != nil {
			return nesko.Ticket{}, fmt.Errorf("finish execution run record: %w", err)
		}
	}

	w.logger.Info("ticket execution finished", "ticket", nesko.FormatTicketReference(input.Ticket), "run_id", input.RunID, "status", status)
	return w.tickets.Get(ctx, input.Ticket.ID)
}

func (w *TicketWorkflow) failExecution(ctx context.Context, input nesko.ExecutionInput, cause error) error {
	if w.lifecycle != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := w.lifecycle.FailExecution(persistCtx, input.Ticket.ID, input.RunID, nesko.ExecutionRunFinish{
			Status:        nesko.ExecutionRunFailed,
			At:            time.Now().UTC(),
			FailureReason: cause.Error(),
		}); err != nil {
			cause = errors.Join(cause, fmt.Errorf("record failed execution: %w", err))
		}
		w.logger.Error("ticket workflow failed", "ticket_id", input.Ticket.ID, "error", cause)
		return cause
	}
	if w.runs != nil {
		if err := w.finishExecutionRun(ctx, input.RunID, nesko.ExecutionRunFinish{
			Status:        nesko.ExecutionRunFailed,
			At:            time.Now().UTC(),
			FailureReason: cause.Error(),
		}); err != nil {
			cause = errors.Join(cause, fmt.Errorf("finish failed execution run: %w", err))
		}
	}
	return w.fail(ctx, input.Ticket.ID, nesko.TicketStatusRunning, cause)
}

func (w *TicketWorkflow) finishExecutionRun(ctx context.Context, runID string, finish nesko.ExecutionRunFinish) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return w.runs.FinishExecutionRun(persistCtx, runID, finish)
}

func (w *TicketWorkflow) recordRunEvent(ctx context.Context, runID string, event nesko.ExecutionEvent) error {
	if w.runs == nil {
		return nil
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	return w.runs.RecordExecutionEvent(ctx, runID, event)
}

func (w *TicketWorkflow) releaseWorkspace(ctx context.Context, input nesko.ExecutionInput, result nesko.ExecutionResult, executionErr error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return w.workspaces.Release(cleanupCtx, input, result, executionErr)
}

func (w *TicketWorkflow) assess(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	ticket, err := w.tickets.Get(ctx, ticketID)
	if err != nil {
		return nesko.Ticket{}, err
	}
	assessment, err := w.judge.Assess(ctx, ticket)
	if err != nil {
		return nesko.Ticket{}, w.fail(ctx, ticketID, nesko.TicketStatusTriaging, err)
	}
	if err := assessment.Validate(); err != nil {
		return nesko.Ticket{}, w.fail(ctx, ticketID, nesko.TicketStatusTriaging, err)
	}
	policyDecision, err := w.policy.Decide(ticket, assessment)
	if err != nil {
		return nesko.Ticket{}, w.fail(ctx, ticketID, nesko.TicketStatusTriaging, fmt.Errorf("apply ticket policy: %w", err))
	}
	decision := nesko.TicketDecision{Assessment: assessment, Policy: policyDecision}
	if err := decision.Validate(); err != nil {
		return nesko.Ticket{}, w.fail(ctx, ticketID, nesko.TicketStatusTriaging, err)
	}
	status := policyDecision.TriageStatus()
	if err := w.tickets.Transition(ctx, ticketID, nesko.TicketTransition{
		From:     nesko.TicketStatusTriaging,
		To:       status,
		At:       time.Now().UTC(),
		Decision: &decision,
	}); err != nil {
		return nesko.Ticket{}, err
	}
	return w.tickets.Get(ctx, ticketID)
}

func (w *TicketWorkflow) fail(ctx context.Context, ticketID string, from nesko.TicketStatus, cause error) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	transitionErr := w.tickets.Transition(persistCtx, ticketID, nesko.TicketTransition{
		From:          from,
		To:            nesko.TicketStatusFailed,
		At:            time.Now().UTC(),
		FailureReason: cause.Error(),
	})
	if transitionErr != nil {
		cause = errors.Join(cause, fmt.Errorf("record ticket failure: %w", transitionErr))
	}
	w.logger.Error("ticket workflow failed", "ticket_id", ticketID, "error", cause)
	return cause
}

func statusesForOutcome(outcome nesko.ExecutionOutcome) (nesko.TicketStatus, nesko.ExecutionRunStatus, error) {
	switch outcome {
	case nesko.ExecutionOutcomeCompleted:
		return nesko.TicketStatusCompleted, nesko.ExecutionRunCompleted, nil
	case nesko.ExecutionOutcomeNeedsReview:
		return nesko.TicketStatusNeedsReview, nesko.ExecutionRunNeedsReview, nil
	case nesko.ExecutionOutcomeWaitingHuman:
		return nesko.TicketStatusWaitingHuman, nesko.ExecutionRunWaitingHuman, nil
	default:
		return "", "", fmt.Errorf("unsupported executor outcome %q", outcome)
	}
}

var _ ports.TicketWorkflow = (*TicketWorkflow)(nil)
