package loop

import (
	"github.com/opus-domini/praetor/internal/orchestration/pipeline"
)

// Runner delegates to pipeline.Runner.
type Runner = pipeline.Runner

// NewRunner delegates to pipeline.NewRunner.
func NewRunner(runtime AgentRuntime) *Runner {
	return pipeline.NewRunner(runtime)
}
