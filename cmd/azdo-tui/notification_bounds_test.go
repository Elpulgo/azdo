package main

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
)

// TestAzureLookbackDaysMax_ConfigAndAdapterAgree pins the two independent
// declarations of the notifications.azure.lookback_days upper bound against
// each other. internal/config does not import internal/azdevops (and
// internal/azdevops does not import internal/config), so nothing in the
// build or the type system otherwise relates config.AzureLookbackDaysMax to
// azdevops.MaxNotificationLookbackDays -- this package is the first one on
// the import graph that sees both, which is why the assertion lives here
// rather than in either leaf package (task-11 review finding 3b).
func TestAzureLookbackDaysMax_ConfigAndAdapterAgree(t *testing.T) {
	if config.AzureLookbackDaysMax != azdevops.MaxNotificationLookbackDays {
		t.Errorf("config.AzureLookbackDaysMax = %d, azdevops.MaxNotificationLookbackDays = %d, want equal",
			config.AzureLookbackDaysMax, azdevops.MaxNotificationLookbackDays)
	}
}

// TestDefaultAzureLookbackDays_ConfigAndAdapterAgree is
// TestAzureLookbackDaysMax_ConfigAndAdapterAgree's sibling for the default
// (rather than the maximum): config.DefaultAzureLookbackDays and
// azdevops.DefaultNotificationLookbackDays are two independent declarations
// of the same 14-day fallback and must be changed together.
func TestDefaultAzureLookbackDays_ConfigAndAdapterAgree(t *testing.T) {
	if config.DefaultAzureLookbackDays != azdevops.DefaultNotificationLookbackDays {
		t.Errorf("config.DefaultAzureLookbackDays = %d, azdevops.DefaultNotificationLookbackDays = %d, want equal",
			config.DefaultAzureLookbackDays, azdevops.DefaultNotificationLookbackDays)
	}
}
