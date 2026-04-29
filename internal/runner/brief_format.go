package runner

import "fmt"

func briefSummary(mustInclude []BriefItem, candidates []BriefItem, healthFootnote string) string {
	total := len(mustInclude) + len(candidates)
	if total == 0 && healthFootnote == "" {
		return "NO_REPLY"
	}
	return fmt.Sprintf("must_include=%d candidates=%d health_footnote=%t", len(mustInclude), len(candidates), healthFootnote != "")
}

func rejectedBrief(paths Paths, reason string) BriefTaskResult {
	return BriefTaskResult{
		Rejected:        true,
		RejectionReason: reason,
		Paths:           paths,
		Summary:         reason,
	}
}
