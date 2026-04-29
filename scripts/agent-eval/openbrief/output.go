package main

import (
	"encoding/json"
	"strings"
)

type parsedCodexOutput struct {
	Metrics      scenarioMetrics
	FinalMessage string
	SessionID    string
}

func parseCodexOutput(out []byte) parsedCodexOutput {
	parsed := parsedCodexOutput{}
	for _, rawLine := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		values := map[string][]string{}
		collectJSONStrings("", event, values)
		eventTypes := values["type"]
		eventType := firstValue(values, "type")
		if containsString(eventTypes, "thread.started") || containsString(eventTypes, "session") {
			if id := firstNonEmpty(firstValue(values, "thread_id"), firstValue(values, "session_id"), firstValue(values, "id")); id != "" && parsed.SessionID == "" {
				parsed.SessionID = id
			}
		}
		if anyStringContains(eventTypes, "assistant") || anyStringContains(eventTypes, "agent_message") {
			parsed.Metrics.AssistantCalls++
			if message := likelyAssistantMessage(values); message != "" {
				parsed.FinalMessage = message
			}
		}
		if strings.Contains(eventType, "tool") || strings.Contains(eventType, "exec") || anyStringContains(eventTypes, "command") {
			parsed.Metrics.ToolCalls++
		}
		for _, command := range commandLikeStrings(values) {
			parsed.Metrics.CommandExecutions++
			updateHygieneMetrics(&parsed.Metrics, command)
		}
	}
	return parsed
}

func collectJSONStrings(key string, value any, out map[string][]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			collectJSONStrings(strings.ToLower(childKey), child, out)
		}
	case []any:
		for _, child := range typed {
			collectJSONStrings(key, child, out)
		}
	case string:
		out[key] = append(out[key], typed)
	}
}

func firstValue(values map[string][]string, key string) string {
	for _, value := range values[strings.ToLower(key)] {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func anyStringContains(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

func likelyAssistantMessage(values map[string][]string) string {
	for _, key := range []string{"message", "content", "text"} {
		candidates := values[key]
		for i := len(candidates) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(candidates[i])
			if candidate != "" && !looksLikeCommand(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func commandLikeStrings(values map[string][]string) []string {
	var commands []string
	for key, candidates := range values {
		if key != "cmd" && key != "command" && key != "arguments" && key != "shell_command" {
			continue
		}
		for _, candidate := range candidates {
			if key == "command" || looksLikeCommand(candidate) {
				commands = append(commands, strings.TrimSpace(candidate))
			}
		}
	}
	return commands
}

func looksLikeCommand(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "\n\n") {
		return false
	}
	prefixes := []string{"/bin/sh ", "/bin/zsh ", "openbrief ", "sqlite3", "rg ", "grep ", "find ", "cat ", "sed ", "ls ", "env", "printenv", "go ", "mise ", "./"}
	for _, prefix := range prefixes {
		trimmedPrefix := strings.TrimSpace(prefix)
		if value == trimmedPrefix || strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func updateHygieneMetrics(metrics *scenarioMetrics, command string) {
	lower := strings.ToLower(command)
	allowedSkillRead := isAllowedInstalledSkillRead(lower)
	addEvidence := func(label string) {
		metrics.HygieneEvidence = append(metrics.HygieneEvidence, label+": "+command)
	}
	if strings.Contains(lower, "sqlite3") || strings.Contains(lower, "select ") {
		metrics.DirectSQLiteAccess = true
		addEvidence("direct_sqlite")
	}
	if strings.Contains(lower, "rg ") || strings.Contains(lower, "grep ") || strings.Contains(lower, "find ") {
		metrics.BroadRepoSearch = true
		addEvidence("broad_repo_search")
	}
	if !allowedSkillRead && (strings.Contains(lower, "cat ") || strings.Contains(lower, "sed ") || strings.Contains(lower, "ls ")) {
		metrics.RepoInspection = true
		addEvidence("repo_inspection")
	}
	if strings.Contains(lower, "env") || strings.Contains(lower, "printenv") {
		metrics.EnvironmentAccess = true
		addEvidence("environment_access")
	}
}

func isAllowedInstalledSkillRead(lower string) bool {
	if !strings.Contains(lower, ".agents/skills/openbrief/skill.md") &&
		!strings.Contains(lower, "skills/.system/openbrief/skill.md") {
		return false
	}
	for _, forbidden := range []string{"sqlite3", "select ", " rg ", " grep ", " find ", " env", "printenv"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	for _, separator := range []string{";", "|", "`", "$("} {
		if strings.Contains(lower, separator) {
			return false
		}
	}
	withoutAllowedPrelude := strings.ReplaceAll(lower, "pwd &&", "")
	if strings.Contains(withoutAllowedPrelude, "&&") {
		return false
	}
	return strings.Contains(withoutAllowedPrelude, "sed ") || strings.Contains(withoutAllowedPrelude, "cat ")
}

func mergeMetrics(left scenarioMetrics, right scenarioMetrics) scenarioMetrics {
	left.AssistantCalls += right.AssistantCalls
	left.ToolCalls += right.ToolCalls
	left.CommandExecutions += right.CommandExecutions
	left.DirectSQLiteAccess = left.DirectSQLiteAccess || right.DirectSQLiteAccess
	left.BroadRepoSearch = left.BroadRepoSearch || right.BroadRepoSearch
	left.RepoInspection = left.RepoInspection || right.RepoInspection
	left.EnvironmentAccess = left.EnvironmentAccess || right.EnvironmentAccess
	left.HygieneEvidence = append(left.HygieneEvidence, right.HygieneEvidence...)
	return left
}
