package loop

import (
	"github.com/opus-domini/praetor/internal/orchestration/pipeline"
)

// PlanRequest delegates to pipeline.PlanRequest.
type PlanRequest = pipeline.PlanRequest

// ExecuteRequest delegates to pipeline.ExecuteRequest.
type ExecuteRequest = pipeline.ExecuteRequest

// ReviewRequest delegates to pipeline.ReviewRequest.
type ReviewRequest = pipeline.ReviewRequest

// CognitiveAgent delegates to pipeline.CognitiveAgent.
type CognitiveAgent = pipeline.CognitiveAgent

// NewCognitiveAgent delegates to pipeline.NewCognitiveAgent.
func NewCognitiveAgent(id Agent, runtime AgentRuntime) (CognitiveAgent, error) {
	return pipeline.NewCognitiveAgent(id, runtime)
}
