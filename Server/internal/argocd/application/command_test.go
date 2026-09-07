package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOnboardingCommandSeparatesOperationsAndConfirmVersions(t *testing.T) {
	now := time.Date(2026, 9, 2, 6, 0, 0, 0, time.UTC)
	actorID, applicationID := uuid.New(), uuid.New()
	version := uint64(7)
	dryRun, err := NewOnboardingCommand(actorID, applicationID, CommandDryRun, "key-1", nil, now)
	if err != nil {
		t.Fatalf("create dry-run command: %v", err)
	}
	confirm, err := NewOnboardingCommand(actorID, applicationID, CommandConfirm, "key-1", &version, now)
	if err != nil {
		t.Fatalf("create confirm command: %v", err)
	}
	if dryRun.RequestFingerprint == confirm.RequestFingerprint || dryRun.State != CommandAccepted || confirm.RequestFingerprint == "" {
		t.Fatalf("command identity was not separated: %#v %#v", dryRun, confirm)
	}
	if _, err := NewOnboardingCommand(actorID, applicationID, CommandConfirm, "key-1", nil, now); err == nil {
		t.Fatal("confirm command without a version should be rejected")
	}
}
