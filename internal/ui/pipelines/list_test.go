package pipelines

import (
	"strings"
	"testing"
)

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		result         string
		wantContains   string
		wantNotContain string
	}{
		// In progress status
		{
			name:         "inProgress status shows Running",
			status:       "inProgress",
			result:       "",
			wantContains: "Running",
		},
		{
			name:         "InProgress (capitalized) shows Running",
			status:       "InProgress",
			result:       "",
			wantContains: "Running",
		},

		// Not started status
		{
			name:         "notStarted status shows Queued",
			status:       "notStarted",
			result:       "",
			wantContains: "Queued",
		},
		{
			name:         "NotStarted (capitalized) shows Queued",
			status:       "NotStarted",
			result:       "",
			wantContains: "Queued",
		},

		// Canceling status
		{
			name:         "canceling status shows Cancel",
			status:       "canceling",
			result:       "",
			wantContains: "Cancel",
		},

		// Result-based status (completed builds)
		{
			name:         "succeeded result shows Success",
			status:       "completed",
			result:       "succeeded",
			wantContains: "Success",
		},
		{
			name:         "failed result shows Failed",
			status:       "completed",
			result:       "failed",
			wantContains: "Failed",
		},
		{
			name:         "canceled result shows Canceled",
			status:       "completed",
			result:       "canceled",
			wantContains: "Canceled",
		},
		{
			name:         "partiallySucceeded result shows Partial",
			status:       "completed",
			result:       "partiallySucceeded",
			wantContains: "Partial",
		},

		// Unknown/default cases - now shows status/result for debugging
		{
			name:         "empty status and result shows debug format",
			status:       "",
			result:       "",
			wantContains: "/",
		},
		{
			name:         "unrecognized status shows debug format",
			status:       "somethingElse",
			result:       "",
			wantContains: "somethingElse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIcon(tt.status, tt.result)

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIcon(%q, %q) = %q, want to contain %q",
					tt.status, tt.result, got, tt.wantContains)
			}

			if tt.wantNotContain != "" && strings.Contains(got, tt.wantNotContain) {
				t.Errorf("statusIcon(%q, %q) = %q, should NOT contain %q",
					tt.status, tt.result, got, tt.wantNotContain)
			}
		})
	}
}
