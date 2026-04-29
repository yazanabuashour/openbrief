package main

type runOptions struct {
	RunRoot    string
	Scenario   string
	ReportDir  string
	ReportName string
}

type runResult struct {
	RunRoot        string      `json:"run_root"`
	CodexHome      string      `json:"codex_home"`
	ScenarioCount  int         `json:"scenario_count"`
	SessionFiles   int         `json:"new_session_files"`
	ScenarioResult []jobResult `json:"scenario_results"`
	ElapsedSeconds float64     `json:"elapsed_seconds"`
}

type jobResult struct {
	ScenarioID   string             `json:"scenario_id"`
	RunDir       string             `json:"run_dir"`
	Database     string             `json:"database"`
	Passed       bool               `json:"passed"`
	Error        string             `json:"error,omitempty"`
	Seconds      float64            `json:"seconds"`
	Metrics      scenarioMetrics    `json:"metrics"`
	Verification verificationResult `json:"verification"`
	FinalMessage string             `json:"final_message,omitempty"`
}

type scenarioMetrics struct {
	AssistantCalls     int      `json:"assistant_calls"`
	ToolCalls          int      `json:"tool_calls"`
	CommandExecutions  int      `json:"command_executions"`
	DirectSQLiteAccess bool     `json:"direct_sqlite_access"`
	BroadRepoSearch    bool     `json:"broad_repo_search"`
	RepoInspection     bool     `json:"repo_inspection"`
	EnvironmentAccess  bool     `json:"environment_access"`
	HygieneEvidence    []string `json:"hygiene_evidence,omitempty"`
}

type verificationResult struct {
	Passed        bool   `json:"passed"`
	DatabasePass  bool   `json:"database_pass"`
	AssistantPass bool   `json:"assistant_pass"`
	Details       string `json:"details,omitempty"`
}
