package service

import "github.com/xnlemon/nesko"

// DecisionPolicy selects a handling route from an assessment and ticket context.
type DecisionPolicy interface {
	Decide(ticket nesko.Ticket, assessment nesko.JudgeAssessment) (nesko.PolicyDecision, error)
}

// DefaultDecisionPolicy applies conservative routing rules to Judge assessments.
type DefaultDecisionPolicy struct{}

// Decide sends uncertain or high-risk assessments to a human, medium cases to approval,
// and only fully evidenced low-risk cases to automatic handling.
func (DefaultDecisionPolicy) Decide(_ nesko.Ticket, assessment nesko.JudgeAssessment) (nesko.PolicyDecision, error) {
	if err := assessment.Validate(); err != nil {
		return nesko.PolicyDecision{}, err
	}
	if hasUnknownAssessment(assessment) {
		return policyDecision(nesko.HandlingHumanInLoop, "assessment contains unknown dimensions and needs human triage"), nil
	}
	if assessment.Confidence == nesko.AssessmentConfidenceLow || len(assessment.Evidence) == 0 {
		return policyDecision(nesko.HandlingHumanInLoop, "assessment confidence or supporting evidence is insufficient for automatic routing"), nil
	}
	if assessment.Complexity == nesko.ComplexityHigh || hasHighRisk(assessment) ||
		assessment.Risks.Reversibility == nesko.ReversibilityHard {
		return policyDecision(nesko.HandlingHumanInLoop, "high complexity, risk, or rollback difficulty needs human review"), nil
	}
	if assessment.AllowsAutomaticHandling() {
		return policyDecision(nesko.HandlingAutomatic, "all risk dimensions are low with high-confidence supporting evidence"), nil
	}
	return policyDecision(nesko.HandlingRequiresApproval, "medium complexity, risk, confidence, or rollback effort needs approval"), nil
}

func hasUnknownAssessment(assessment nesko.JudgeAssessment) bool {
	return assessment.Complexity == nesko.ComplexityUnknown ||
		assessment.Risks.Compatibility == nesko.RiskLevelUnknown ||
		assessment.Risks.DataImpact == nesko.RiskLevelUnknown ||
		assessment.Risks.SecurityImpact == nesko.RiskLevelUnknown ||
		assessment.Risks.OperationalImpact == nesko.RiskLevelUnknown ||
		assessment.Risks.BlastRadius == nesko.RiskLevelUnknown ||
		assessment.Risks.Reversibility == nesko.ReversibilityUnknown ||
		assessment.Confidence == nesko.AssessmentConfidenceUnknown
}

func hasHighRisk(assessment nesko.JudgeAssessment) bool {
	return assessment.Risks.Compatibility == nesko.RiskLevelHigh ||
		assessment.Risks.DataImpact == nesko.RiskLevelHigh ||
		assessment.Risks.SecurityImpact == nesko.RiskLevelHigh ||
		assessment.Risks.OperationalImpact == nesko.RiskLevelHigh ||
		assessment.Risks.BlastRadius == nesko.RiskLevelHigh
}

func policyDecision(mode nesko.HandlingMode, rationale string) nesko.PolicyDecision {
	return nesko.PolicyDecision{Mode: mode, Rationale: rationale}
}
