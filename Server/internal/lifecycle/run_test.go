package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/vincent119/zlogger"

	"github.com/vincent119/ReleaseHub/Server/internal/lifecycle"
)

func TestRunExecutesCleanupAfterTaskStops(t *testing.T) {
	cleaned := false
	err := lifecycle.Run(
		func(context.Context) error { return nil },
		100*time.Millisecond,
		zlogger.NewNop(),
		func(context.Context) error {
			cleaned = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("lifecycle exited with error: %v", err)
	}
	if !cleaned {
		t.Fatal("cleanup should run after the primary task stops")
	}
}
