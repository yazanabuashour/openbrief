package runner

import (
	"context"
	"testing"
)

func TestRunBriefRejectsNoEnabledSources(t *testing.T) {
	result, err := RunBriefTask(context.Background(), testConfig(t), BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if !result.Rejected || result.RejectionReason != "no enabled sources configured" {
		t.Fatalf("result = %+v", result)
	}
}
