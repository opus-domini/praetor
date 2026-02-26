package loop

import (
	"io"

	localstate "github.com/opus-domini/praetor/internal/state"
)

// MigrateLegacyState copies legacy ~/.praetor data to XDG-compliant locations.
func MigrateLegacyState(out io.Writer, dryRun bool) error {
	return localstate.MigrateLegacyState(out, dryRun)
}
