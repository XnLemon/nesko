package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/xnlemon/nesko/ports"
)

type settledEvent struct {
	aborted bool
}

func (e *Executor) waitForAgent(ctx context.Context, lines <-chan rpcLine, promptID string, report ports.ExecutionReporter) (settledEvent, bool, error) {
	ticker := time.NewTicker(e.heartbeatInterval)
	defer ticker.Stop()
	promptAccepted := false
	for {
		select {
		case <-ctx.Done():
			return settledEvent{}, promptAccepted, ctx.Err()
		case <-ticker.C:
			if err := reportHeartbeat(report); err != nil {
				return settledEvent{}, promptAccepted, err
			}
		case line, ok := <-lines:
			if !ok {
				return settledEvent{}, promptAccepted, errors.New("pi RPC stdout closed before the agent settled")
			}
			if line.err != nil {
				return settledEvent{}, promptAccepted, fmt.Errorf("read Pi RPC record: %w", line.err)
			}
			record, err := decodeRPCRecord(line.data)
			if err != nil {
				return settledEvent{}, promptAccepted, err
			}
			accepted, settled, err := handleAgentRecord(record, promptID, promptAccepted, report)
			if err != nil {
				return settledEvent{}, accepted, err
			}
			if settled != nil {
				return *settled, accepted, nil
			}
			promptAccepted = accepted
		}
	}
}

func handleAgentRecord(record rpcRecord, promptID string, promptAccepted bool, report ports.ExecutionReporter) (bool, *settledEvent, error) {
	switch record.Type {
	case "response":
		accepted, err := handlePromptResponse(record, promptID, promptAccepted, report)
		return accepted, nil, err
	case "agent_start":
		return true, nil, nil
	case "agent_settled":
		return promptAccepted, &settledEvent{aborted: record.Aborted}, nil
	case "tool_execution_start":
		if err := handleToolStart(record, report); err != nil {
			return true, nil, err
		}
		return true, nil, nil
	case "tool_execution_end":
		if err := handleToolEnd(record, report); err != nil {
			return promptAccepted, nil, err
		}
	case "extension_error":
		return promptAccepted, nil, fmt.Errorf("pi extension error: %s", record.Error)
	}
	return promptAccepted, nil, nil
}

func handlePromptResponse(record rpcRecord, promptID string, promptAccepted bool, report ports.ExecutionReporter) (bool, error) {
	if record.ID != promptID {
		return promptAccepted, nil
	}
	if !record.Success {
		return false, fmt.Errorf("pi rejected the prompt: %s", record.Error)
	}
	disposition, err := responseDisposition(record.Data)
	if err != nil {
		return false, err
	}
	if disposition == "handled" {
		return false, errors.New("pi handled the prompt without starting an agent run")
	}
	if err := reportProgress(report, "Pi began processing the ticket"); err != nil {
		return true, err
	}
	return true, nil
}

func handleToolStart(record rpcRecord, report ports.ExecutionReporter) error {
	return reportProgress(report, "Pi started tool "+safeDetail(record.ToolName))
}

func handleToolEnd(record rpcRecord, report ports.ExecutionReporter) error {
	detail := "Pi finished tool " + safeDetail(record.ToolName)
	if record.IsError {
		detail += " with an error"
	}
	return reportProgress(report, detail)
}

func (e *Executor) readFinalText(ctx context.Context, lines <-chan rpcLine, responseID string, report ports.ExecutionReporter) (string, error) {
	ticker := time.NewTicker(e.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if err := reportHeartbeat(report); err != nil {
				return "", err
			}
		case line, ok := <-lines:
			if !ok {
				return "", errors.New("pi RPC stdout closed before returning its final response")
			}
			if line.err != nil {
				return "", fmt.Errorf("read Pi final response: %w", line.err)
			}
			record, err := decodeRPCRecord(line.data)
			if err != nil {
				return "", err
			}
			text, matched, err := finalTextResponse(record, responseID)
			if err != nil {
				return "", err
			}
			if matched {
				return text, nil
			}
		}
	}
}

func finalTextResponse(record rpcRecord, responseID string) (string, bool, error) {
	if record.Type != "response" || record.ID != responseID {
		return "", false, nil
	}
	if !record.Success {
		return "", true, fmt.Errorf("pi could not return its final response: %s", record.Error)
	}
	var data struct {
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(record.Data, &data); err != nil {
		return "", true, fmt.Errorf("decode Pi final response: %w", err)
	}
	if data.Text == nil {
		return "", true, nil
	}
	return *data.Text, true, nil
}
