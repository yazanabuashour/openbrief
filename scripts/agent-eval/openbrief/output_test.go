package main

import (
	"strings"
	"testing"
)

func TestFailedScenarioErrorReportsAnyFailure(t *testing.T) {
	if err := failedScenarioError([]jobResult{{Passed: true}, {Passed: true}}); err != nil {
		t.Fatalf("failedScenarioError() = %v, want nil", err)
	}
	if err := failedScenarioError([]jobResult{{Passed: true}, {Passed: false}}); err == nil {
		t.Fatal("failedScenarioError() = nil, want error")
	}
}

func TestParseCodexOutputDetectsHygieneAndFinalMessage(t *testing.T) {
	out := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-123"}`,
		`{"type":"tool.call","cmd":"sqlite3 openbrief.sqlite 'select * from brief_source'"}`,
		`{"type":"assistant.message","message":"NO_REPLY"}`,
	}, "\n"))

	parsed := parseCodexOutput(out)
	if parsed.SessionID != "thread-123" {
		t.Fatalf("SessionID = %q", parsed.SessionID)
	}
	if parsed.FinalMessage != "NO_REPLY" {
		t.Fatalf("FinalMessage = %q", parsed.FinalMessage)
	}
	if parsed.Metrics.AssistantCalls != 1 || !parsed.Metrics.DirectSQLiteAccess || parsed.Metrics.CommandExecutions != 1 {
		t.Fatalf("metrics = %+v", parsed.Metrics)
	}
}

func TestParseCodexOutputAllowsInstalledSkillRead(t *testing.T) {
	out := []byte(`{"type":"item.started","item":{"type":"command_execution","command":"/bin/zsh -lc \"pwd && sed -n '1,220p' .agents/skills/openbrief/SKILL.md\""}}`)

	parsed := parseCodexOutput(out)
	if parsed.Metrics.RepoInspection || len(parsed.Metrics.HygieneEvidence) != 0 {
		t.Fatalf("metrics = %+v, want installed skill read allowed", parsed.Metrics)
	}
}

func TestParseCodexOutputFlagsCompoundCommandAfterSkillRead(t *testing.T) {
	out := []byte(`{"type":"item.started","item":{"type":"command_execution","command":"/bin/zsh -lc \"sed -n '1,220p' .agents/skills/openbrief/SKILL.md && sqlite3 openbrief.sqlite 'select * from brief_source'\""}}`)

	parsed := parseCodexOutput(out)
	if !parsed.Metrics.DirectSQLiteAccess || !parsed.Metrics.RepoInspection {
		t.Fatalf("metrics = %+v, want compound skill read and sqlite command flagged", parsed.Metrics)
	}
}
