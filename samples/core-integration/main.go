package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/adapters/sqlite"
	"github.com/xnlemon/nesko/ports"
	"github.com/xnlemon/nesko/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	directory, err := os.MkdirTemp("", "nesko-core-integration-")
	if err != nil {
		return fmt.Errorf("create integration sample directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(directory); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("remove integration sample directory: %w", err))
		}
	}()
	repository, err := sqlite.NewTicketRepository(filepath.Join(directory, "tickets.sqlite"))
	if err != nil {
		return fmt.Errorf("open integration sample repository: %w", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close integration sample repository: %w", err))
		}
	}()

	intake, err := service.NewReceiveRequest(repository, sampleReplier{}, nil)
	if err != nil {
		return fmt.Errorf("configure integration sample intake: %w", err)
	}
	workflow, err := service.NewTicketWorkflow(service.TicketWorkflowDependencies{
		Tickets:   repository,
		Judge:     sampleJudge{},
		Router:    ports.ExecutorRouterFunc(sampleRoute),
		Runs:      repository,
		Lifecycle: repository,
	})
	if err != nil {
		return fmt.Errorf("configure integration sample workflow: %w", err)
	}

	ctx := context.Background()
	completed, err := runCompletedFlow(ctx, intake, repository, workflow)
	if err != nil {
		return err
	}
	interrupted, err := runStaleRecovery(ctx, intake, repository, workflow)
	if err != nil {
		return err
	}
	output, err := json.MarshalIndent(struct {
		CompletedTicketStatus nesko.TicketStatus       `json:"completed_ticket_status"`
		CompletedRunStatus    nesko.ExecutionRunStatus `json:"completed_run_status"`
		CompletedEventCount   int                      `json:"completed_event_count"`
		InterruptedTicket     nesko.TicketStatus       `json:"interrupted_ticket_status"`
		InterruptedRun        nesko.ExecutionRunStatus `json:"interrupted_run_status"`
	}{
		CompletedTicketStatus: completed.ticketStatus,
		CompletedRunStatus:    completed.runStatus,
		CompletedEventCount:   completed.eventCount,
		InterruptedTicket:     interrupted.ticketStatus,
		InterruptedRun:        interrupted.runStatus,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode integration sample result: %w", err)
	}
	fmt.Println(string(output))
	return nil
}

type completedFlowSummary struct {
	ticketStatus nesko.TicketStatus
	runStatus    nesko.ExecutionRunStatus
	eventCount   int
}

func runCompletedFlow(ctx context.Context, intake *service.ReceiveRequest, repository *sqlite.TicketRepository, workflow ports.TicketWorkflow) (completedFlowSummary, error) {
	ticket, err := intake.Receive(ctx, sampleRequest("bounded-change"))
	if err != nil {
		return completedFlowSummary{}, fmt.Errorf("receive completion-flow request: %w", err)
	}
	ticket, err = workflow.Assess(ctx, ticket.ID)
	if err != nil {
		return completedFlowSummary{}, fmt.Errorf("assess completion-flow ticket: %w", err)
	}
	if ticket.Status != nesko.TicketStatusQueued {
		return completedFlowSummary{}, fmt.Errorf("expected an automatically queued ticket, got %s", ticket.Status)
	}
	ticket, err = workflow.Execute(ctx, ticket.ID)
	if err != nil {
		return completedFlowSummary{}, fmt.Errorf("execute completion-flow ticket: %w", err)
	}
	run, err := repository.GetExecutionRun(ctx, ticket.ExecutionID)
	if err != nil {
		return completedFlowSummary{}, fmt.Errorf("load completed execution run: %w", err)
	}
	events, err := repository.ListExecutionRunEvents(ctx, run.ID)
	if err != nil {
		return completedFlowSummary{}, fmt.Errorf("load completed execution events: %w", err)
	}
	if ticket.Status != nesko.TicketStatusCompleted || run.Status != nesko.ExecutionRunCompleted || len(events) < 4 {
		return completedFlowSummary{}, errors.New("completion flow did not persist the expected ticket, run, and event states")
	}
	return completedFlowSummary{ticketStatus: ticket.Status, runStatus: run.Status, eventCount: len(events)}, nil
}

type interruptedFlowSummary struct {
	ticketStatus nesko.TicketStatus
	runStatus    nesko.ExecutionRunStatus
}

func runStaleRecovery(ctx context.Context, intake *service.ReceiveRequest, repository *sqlite.TicketRepository, workflow ports.TicketWorkflow) (interruptedFlowSummary, error) {
	ticket, err := intake.Receive(ctx, sampleRequest("stale-run"))
	if err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("receive stale-run request: %w", err)
	}
	ticket, err = workflow.Assess(ctx, ticket.ID)
	if err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("assess stale-run ticket: %w", err)
	}
	if ticket.Status != nesko.TicketStatusQueued {
		return interruptedFlowSummary{}, fmt.Errorf("expected stale-run ticket to be queued, got %s", ticket.Status)
	}
	startedAt := time.Now().UTC().Add(-10 * time.Minute)
	runID := uuid.NewString()
	if _, err := repository.StartExecution(ctx, ticket.ID, runID, startedAt); err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("start stale-run lifecycle: %w", err)
	}
	interruptedAt := time.Now().UTC()
	heartbeatBefore := interruptedAt.Add(-5 * time.Minute)
	const reason = "integration sample detected an expired heartbeat"
	if err := repository.InterruptStaleExecution(ctx, runID, heartbeatBefore, interruptedAt, reason); err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("interrupt stale run: %w", err)
	}
	if err := repository.InterruptStaleExecution(ctx, runID, heartbeatBefore, time.Time{}, reason); err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("repeat stale-run interruption: %w", err)
	}
	ticket, err = repository.Get(ctx, ticket.ID)
	if err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("load interrupted ticket: %w", err)
	}
	run, err := repository.GetExecutionRun(ctx, runID)
	if err != nil {
		return interruptedFlowSummary{}, fmt.Errorf("load interrupted run: %w", err)
	}
	if ticket.Status != nesko.TicketStatusFailed || run.Status != nesko.ExecutionRunInterrupted {
		return interruptedFlowSummary{}, errors.New("stale-run recovery did not update ticket and run atomically")
	}
	return interruptedFlowSummary{ticketStatus: ticket.Status, runStatus: run.Status}, nil
}

func sampleRequest(suffix string) nesko.IncomingRequest {
	return nesko.IncomingRequest{
		Channel:        nesko.ChannelFeishu,
		MessageID:      uuid.NewString(),
		ConversationID: "core-integration-sample",
		RequesterID:    "sample-user",
		Text:           "Please make the bounded sample change: " + suffix,
		ReceivedAt:     time.Now().UTC(),
	}
}

type sampleReplier struct{}

func (sampleReplier) ReplyToMessage(context.Context, ports.ReplyInput) (string, error) {
	return uuid.NewString(), nil
}

type sampleJudge struct{}

func (sampleJudge) Assess(context.Context, nesko.Ticket) (nesko.JudgeAssessment, error) {
	return nesko.JudgeAssessment{
		Complexity: nesko.ComplexityLow,
		Risks: nesko.RiskAssessment{
			Compatibility:     nesko.RiskLevelLow,
			DataImpact:        nesko.RiskLevelLow,
			SecurityImpact:    nesko.RiskLevelLow,
			OperationalImpact: nesko.RiskLevelLow,
			BlastRadius:       nesko.RiskLevelLow,
			Reversibility:     nesko.ReversibilityEasy,
		},
		Confidence: nesko.AssessmentConfidenceHigh,
		Rationale:  "The sample request is bounded and uses a reversible local change.",
		Evidence: []nesko.AssessmentEvidence{{
			Reference: "samples/core-integration",
			Finding:   "The request is isolated to a disposable sample workspace.",
		}},
	}, nil
}

func sampleRoute(context.Context, nesko.ExecutionInput) (ports.Executor, error) {
	return sampleExecutor{}, nil
}

type sampleExecutor struct{}

func (sampleExecutor) Execute(_ context.Context, _ nesko.ExecutionInput, report ports.ExecutionReporter) (nesko.ExecutionResult, error) {
	if err := report(nesko.ExecutionEvent{Type: nesko.ExecutionEventHeartbeat, At: time.Now().UTC()}); err != nil {
		return nesko.ExecutionResult{}, err
	}
	if err := report(nesko.ExecutionEvent{Type: nesko.ExecutionEventProgress, At: time.Now().UTC(), Detail: "sample executor completed its bounded operation"}); err != nil {
		return nesko.ExecutionResult{}, err
	}
	return nesko.ExecutionResult{
		Outcome: nesko.ExecutionOutcomeCompleted,
		Summary: "The integration sample completed the operation.",
	}, nil
}
