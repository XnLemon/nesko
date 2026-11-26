package pi

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

// Execute runs one ticket in the configured workspace and reports Pi progress.
func (e *Executor) Execute(ctx context.Context, input nesko.ExecutionInput, report ports.ExecutionReporter) (result nesko.ExecutionResult, runErr error) {
	if err := validateExecutionInput(input); err != nil {
		return nesko.ExecutionResult{}, err
	}

	workingDirectory := input.Workspace.Directory
	if workingDirectory == "" {
		workingDirectory = e.workingDirectory
	}
	workingDirectory, err := resolveWorkingDirectory(workingDirectory)
	if err != nil {
		return nesko.ExecutionResult{}, err
	}
	session, err := e.start(ctx, workingDirectory)
	if err != nil {
		return nesko.ExecutionResult{}, fmt.Errorf("start Pi RPC session: %w", err)
	}
	defer func() { runErr = closeSession(session, runErr) }()
	if err := submitPrompt(session.stdin, input, report); err != nil {
		return nesko.ExecutionResult{}, err
	}
	settled, promptAccepted, err := e.waitForAgent(ctx, session.stdoutLines, "nesko-prompt", report)
	if err != nil {
		return nesko.ExecutionResult{}, err
	}
	if !promptAccepted {
		return nesko.ExecutionResult{}, errors.New("Pi settled without accepting the ticket prompt")
	}
	if settled.aborted {
		return nesko.ExecutionResult{}, errors.New("Pi aborted the ticket run")
	}
	finalText, err := requestFinalText(ctx, e, session, report)
	if err != nil {
		return nesko.ExecutionResult{}, err
	}
	if err := session.stdin.Close(); err != nil {
		return nesko.ExecutionResult{}, fmt.Errorf("close Pi stdin: %w", err)
	}
	if err := session.wait(ctx, e.shutdownTimeout); err != nil {
		return nesko.ExecutionResult{}, fmt.Errorf("wait for Pi process: %w", err)
	}
	result = parseExecutionResult(finalText)
	if err := reportProgress(report, "Pi settled the ticket run"); err != nil {
		return nesko.ExecutionResult{}, err
	}
	return result, nil
}

func validateExecutionInput(input nesko.ExecutionInput) error {
	if input.RunID == "" || input.Ticket.ID == "" {
		return errors.New("Pi execution requires a ticket ID and run ID")
	}
	if input.Ticket.Decision == nil {
		return errors.New("Pi execution requires a validated ticket decision")
	}
	if err := input.Ticket.Decision.Validate(); err != nil {
		return fmt.Errorf("validate Pi ticket decision: %w", err)
	}
	return nil
}

func closeSession(session *rpcSession, runErr error) error {
	session.close()
	if runErr == nil || session.stderrTail.String() == "" {
		return runErr
	}
	return fmt.Errorf("%w (Pi stderr: %s)", runErr, session.stderrTail.String())
}

func submitPrompt(stdin io.Writer, input nesko.ExecutionInput, report ports.ExecutionReporter) error {
	prompt, err := executionPrompt(input)
	if err != nil {
		return err
	}
	if err := sendCommand(stdin, rpcCommand{ID: "nesko-prompt", Type: "prompt", Message: prompt}); err != nil {
		return fmt.Errorf("send Pi prompt: %w", err)
	}
	if err := reportProgress(report, "Pi RPC prompt submitted"); err != nil {
		return err
	}
	return nil
}

func requestFinalText(ctx context.Context, executor *Executor, session *rpcSession, report ports.ExecutionReporter) (string, error) {
	const finalID = "nesko-final"
	if err := sendCommand(session.stdin, rpcCommand{ID: finalID, Type: "get_last_assistant_text"}); err != nil {
		return "", fmt.Errorf("request Pi final response: %w", err)
	}
	return executor.readFinalText(ctx, session.stdoutLines, finalID, report)
}

var _ ports.Executor = (*Executor)(nil)
