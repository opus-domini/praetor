package loop

import "github.com/opus-domini/praetor/internal/orchestration/pipeline"

// TransitionRecorder delegates to pipeline.TransitionRecorder.
type TransitionRecorder = pipeline.TransitionRecorder

// IsolationPolicy delegates to pipeline.IsolationPolicy.
type IsolationPolicy = pipeline.IsolationPolicy

// NewTransitionRecorder delegates to pipeline.NewTransitionRecorder.
func NewTransitionRecorder(store *Store, planFile string) *TransitionRecorder {
	return pipeline.NewTransitionRecorder(store, planFile)
}

// NewIsolationPolicy delegates to pipeline.NewIsolationPolicy.
func NewIsolationPolicy(mainDir string, mode IsolationMode) *IsolationPolicy {
	return pipeline.NewIsolationPolicy(mainDir, mode)
}

// CaptureGitDiff delegates to pipeline.CaptureGitDiff.
func CaptureGitDiff(workdir string, maxLines int) string {
	return pipeline.CaptureGitDiff(workdir, maxLines)
}
