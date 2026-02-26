package loop

import "github.com/opus-domini/praetor/internal/domain"

// RenderSink is a type alias for domain.RenderSink.
// It decouples the orchestration loop from the concrete renderer in cli/.
type RenderSink = domain.RenderSink
