package nesko

import (
	"errors"
	"strings"
)

// Complexity estimates the effort and uncertainty of a requested change.
type Complexity string

const (
	ComplexityUnknown Complexity = "unknown"
	ComplexityLow     Complexity = "low"
	ComplexityMedium  Complexity = "medium"
	ComplexityHigh    Complexity = "high"
)

// RiskLevel describes the severity of one risk dimension.
type RiskLevel string

const (
	RiskLevelUnknown RiskLevel = "unknown"
	RiskLevelLow     RiskLevel = "low"
	RiskLevelMedium  RiskLevel = "medium"
	RiskLevelHigh    RiskLevel = "high"
)

// Reversibility describes how readily a change can be rolled back.
type Reversibility string

const (
	ReversibilityUnknown    Reversibility = "unknown"
	ReversibilityEasy       Reversibility = "easy"
	ReversibilityControlled Reversibility = "controlled"
	ReversibilityHard       Reversibility = "hard"
)

// AssessmentConfidence describes how well the assessment is supported.
type AssessmentConfidence string

const (
	AssessmentConfidenceUnknown AssessmentConfidence = "unknown"
	AssessmentConfidenceLow     AssessmentConfidence = "low"
	AssessmentConfidenceMedium  AssessmentConfidence = "medium"
	AssessmentConfidenceHigh    AssessmentConfidence = "high"
)

// RiskAssessment separates compatibility, data, security, operations, and impact risks.
type RiskAssessment struct {
	Compatibility     RiskLevel     `json:"compatibility"`
	DataImpact        RiskLevel     `json:"data_impact"`
	SecurityImpact    RiskLevel     `json:"security_impact"`
	OperationalImpact RiskLevel     `json:"operational_impact"`
	BlastRadius       RiskLevel     `json:"blast_radius"`
	Reversibility     Reversibility `json:"reversibility"`
}

// AssessmentEvidence records a concrete observation that supports the assessment.
type AssessmentEvidence struct {
	Reference string `json:"reference"`
	Finding   string `json:"finding"`
}

// JudgeAssessment describes the estimated complexity and risks without choosing a route.
type JudgeAssessment struct {
	Complexity Complexity           `json:"complexity"`
	Risks      RiskAssessment       `json:"risks"`
	Confidence AssessmentConfidence `json:"confidence"`
	Rationale  string               `json:"rationale"`
	Evidence   []AssessmentEvidence `json:"evidence"`
}

// Validate checks that the assessment uses supported values and well-formed evidence.
func (a JudgeAssessment) Validate() error {
	if !validComplexity(a.Complexity) || !validRiskLevel(a.Risks.Compatibility) ||
		!validRiskLevel(a.Risks.DataImpact) || !validRiskLevel(a.Risks.SecurityImpact) ||
		!validRiskLevel(a.Risks.OperationalImpact) || !validRiskLevel(a.Risks.BlastRadius) ||
		!validReversibility(a.Risks.Reversibility) || !validAssessmentConfidence(a.Confidence) {
		return ErrInvalidJudgeAssessment
	}
	if strings.TrimSpace(a.Rationale) == "" {
		return ErrInvalidJudgeAssessment
	}
	for _, evidence := range a.Evidence {
		if strings.TrimSpace(evidence.Reference) == "" || strings.TrimSpace(evidence.Finding) == "" {
			return ErrInvalidJudgeAssessment
		}
	}
	return nil
}

// HandlingMode is the route selected by policy after assessment.
type HandlingMode string

const (
	HandlingAutomatic        HandlingMode = "automatic"
	HandlingRequiresApproval HandlingMode = "requires_approval"
	HandlingHumanInLoop      HandlingMode = "human_in_loop"
)

var (
	ErrInvalidJudgeAssessment = errors.New("invalid judge assessment")
	ErrInvalidPolicyDecision  = errors.New("invalid policy decision")
	ErrUnsafeAutomaticRoute   = errors.New("automatic handling requires low risk and high-confidence evidence")
)

// PolicyDecision records the route selected by policy and its rationale.
type PolicyDecision struct {
	Mode      HandlingMode
	Rationale string
}

// Validate checks that a policy selected a supported route with an explanation.
func (d PolicyDecision) Validate() error {
	if !validHandlingMode(d.Mode) || strings.TrimSpace(d.Rationale) == "" {
		return ErrInvalidPolicyDecision
	}
	return nil
}

// TriageStatus maps the selected route to the next ticket status.
func (d PolicyDecision) TriageStatus() TicketStatus {
	switch d.Mode {
	case HandlingAutomatic:
		return TicketStatusQueued
	case HandlingRequiresApproval:
		return TicketStatusAwaitingApproval
	case HandlingHumanInLoop:
		return TicketStatusWaitingHuman
	default:
		return ""
	}
}

// TicketDecision combines the Judge's assessment and the policy's routing decision.
type TicketDecision struct {
	Assessment JudgeAssessment
	Policy     PolicyDecision
}

// Validate also enforces the automatic-execution safety boundary.
func (d TicketDecision) Validate() error {
	if err := d.Assessment.Validate(); err != nil {
		return err
	}
	if err := d.Policy.Validate(); err != nil {
		return err
	}
	if d.Policy.Mode == HandlingAutomatic && !d.Assessment.AllowsAutomaticHandling() {
		return ErrUnsafeAutomaticRoute
	}
	return nil
}

// AllowsAutomaticHandling reports whether the assessment meets the automatic route gate.
func (a JudgeAssessment) AllowsAutomaticHandling() bool {
	return a.Complexity == ComplexityLow &&
		a.Risks.Compatibility == RiskLevelLow &&
		a.Risks.DataImpact == RiskLevelLow &&
		a.Risks.SecurityImpact == RiskLevelLow &&
		a.Risks.OperationalImpact == RiskLevelLow &&
		a.Risks.BlastRadius == RiskLevelLow &&
		a.Risks.Reversibility == ReversibilityEasy &&
		a.Confidence == AssessmentConfidenceHigh &&
		len(a.Evidence) > 0
}

func validComplexity(value Complexity) bool {
	switch value {
	case ComplexityUnknown, ComplexityLow, ComplexityMedium, ComplexityHigh:
		return true
	default:
		return false
	}
}

func validRiskLevel(value RiskLevel) bool {
	switch value {
	case RiskLevelUnknown, RiskLevelLow, RiskLevelMedium, RiskLevelHigh:
		return true
	default:
		return false
	}
}

func validReversibility(value Reversibility) bool {
	switch value {
	case ReversibilityUnknown, ReversibilityEasy, ReversibilityControlled, ReversibilityHard:
		return true
	default:
		return false
	}
}

func validAssessmentConfidence(value AssessmentConfidence) bool {
	switch value {
	case AssessmentConfidenceUnknown, AssessmentConfidenceLow, AssessmentConfidenceMedium, AssessmentConfidenceHigh:
		return true
	default:
		return false
	}
}

func validHandlingMode(value HandlingMode) bool {
	switch value {
	case HandlingAutomatic, HandlingRequiresApproval, HandlingHumanInLoop:
		return true
	default:
		return false
	}
}
