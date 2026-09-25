package releases

import (
	"log/slog"
	"os"
	"testing"
)

// TestMain drops what the modal logs, which would otherwise go to the
// test output and garble benchmark results.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}
