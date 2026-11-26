package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/xnlemon/nesko"
)

// scanTicket maps nullable legacy columns and the structured assessment into a ticket.
func scanTicket(row *sql.Row) (nesko.Ticket, error) {
	var ticket nesko.Ticket
	var acknowledgementID, complexity, risk, mode, rationale, assessmentJSON, heartbeat sql.NullString
	if err := row.Scan(
		&ticket.Number, &ticket.ID, &ticket.Channel, &ticket.SourceMessageID,
		&ticket.ConversationID, &ticket.RequesterID, &ticket.RequestText,
		&ticket.Status, &ticket.CreatedAt, &ticket.UpdatedAt, &acknowledgementID,
		&complexity, &risk, &mode, &rationale, &assessmentJSON, &ticket.ExecutionID, &heartbeat,
		&ticket.LastProgress, &ticket.ExecutionSummary, &ticket.FailureReason,
	); err != nil {
		return nesko.Ticket{}, err
	}
	if acknowledgementID.Valid {
		ticket.AcknowledgementMessageID = acknowledgementID.String
	}
	decision, err := decodeTicketDecision(decisionRecord{
		complexity: complexity,
		risk:       risk,
		mode:       mode,
		rationale:  rationale,
		assessment: assessmentJSON,
	})
	if err != nil {
		return nesko.Ticket{}, err
	}
	ticket.Decision = decision
	if heartbeat.Valid {
		value, err := time.Parse(time.RFC3339Nano, heartbeat.String)
		if err != nil {
			return nesko.Ticket{}, fmt.Errorf("parse ticket heartbeat: %w", err)
		}
		ticket.LastHeartbeatAt = &value
	}
	return ticket, nil
}

type decisionRecord struct {
	complexity sql.NullString
	risk       sql.NullString
	mode       sql.NullString
	rationale  sql.NullString
	assessment sql.NullString
}

func decodeTicketDecision(record decisionRecord) (*nesko.TicketDecision, error) {
	fields := []sql.NullString{record.complexity, record.risk, record.mode, record.rationale}
	fieldCount := 0
	for _, field := range fields {
		if field.Valid {
			fieldCount++
		}
	}
	if fieldCount == 0 {
		return nil, nil
	}
	if fieldCount != len(fields) {
		return nil, errors.New("ticket has an incomplete persisted decision")
	}

	assessment, err := decodeAssessment(record.assessment.String, record.complexity.String, record.risk.String)
	if err != nil {
		return nil, err
	}
	return &nesko.TicketDecision{
		Assessment: assessment,
		Policy: nesko.PolicyDecision{
			Mode:      nesko.HandlingMode(record.mode.String),
			Rationale: record.rationale.String,
		},
	}, nil
}

func decisionValues(decision *nesko.TicketDecision) (string, string, string, string, string, error) {
	if decision == nil {
		return "", "", "", "", "", nil
	}
	assessmentJSON, err := json.Marshal(decision.Assessment)
	if err != nil {
		return "", "", "", "", "", err
	}
	return string(decision.Assessment.Complexity),
		string(decision.Assessment.Risks.Compatibility),
		string(decision.Policy.Mode), decision.Policy.Rationale, string(assessmentJSON), nil
}

func decodeAssessment(encoded, complexity, compatibility string) (nesko.JudgeAssessment, error) {
	if encoded != "" {
		var assessment nesko.JudgeAssessment
		if err := json.Unmarshal([]byte(encoded), &assessment); err != nil {
			return nesko.JudgeAssessment{}, fmt.Errorf("decode persisted judge assessment: %w", err)
		}
		if string(assessment.Complexity) != complexity || string(assessment.Risks.Compatibility) != compatibility {
			return nesko.JudgeAssessment{}, errors.New("persisted assessment does not match indexed decision fields")
		}
		return assessment, nil
	}
	return legacyAssessment(complexity, compatibility), nil
}

func legacyAssessment(complexity, compatibility string) nesko.JudgeAssessment {
	risk := nesko.RiskLevelUnknown
	switch compatibility {
	case "non_breaking":
		risk = nesko.RiskLevelLow
	case "breaking":
		risk = nesko.RiskLevelHigh
	case "low", "medium", "high", "unknown":
		risk = nesko.RiskLevel(compatibility)
	}
	return nesko.JudgeAssessment{
		Complexity: nesko.Complexity(complexity),
		Risks: nesko.RiskAssessment{
			Compatibility:     risk,
			DataImpact:        nesko.RiskLevelUnknown,
			SecurityImpact:    nesko.RiskLevelUnknown,
			OperationalImpact: nesko.RiskLevelUnknown,
			BlastRadius:       nesko.RiskLevelUnknown,
			Reversibility:     nesko.ReversibilityUnknown,
		},
		Confidence: nesko.AssessmentConfidenceUnknown,
		Rationale:  "assessment details were recorded by an earlier version",
	}
}
