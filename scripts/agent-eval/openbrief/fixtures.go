package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func prepareScenarioFixtures(current scenario, runDir string) (scenario, error) {
	needsFeed := scenarioContains(current, "https://github.blog/feed/")
	needsReleases := scenarioContains(current, "repository openai/codex")
	needsLimitFeeds := current.ID == "configured-max-delivery-items"
	if !needsFeed && !needsReleases && !needsLimitFeeds {
		return current, nil
	}
	fixtureDir := filepath.Join(runDir, "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		return scenario{}, err
	}
	feedPath := filepath.Join(fixtureDir, "github-blog.xml")
	releasePath := filepath.Join(fixtureDir, "codex-releases.json")
	feed := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>OpenBrief fixture</title>
<item><title>OpenBrief fixture story - Fixture Outlet</title><link>https://fixture.example/story</link><guid>fixture-guid-1</guid><pubDate>Thu, 23 Apr 2026 01:00:00 GMT</pubDate></item>
</channel></rss>`
	if needsFeed {
		if err := os.WriteFile(feedPath, []byte(feed), 0o644); err != nil {
			return scenario{}, err
		}
	}
	var limitFeedURLs []string
	if needsLimitFeeds {
		for i := 1; i <= 3; i++ {
			path := filepath.Join(fixtureDir, fmt.Sprintf("limit-%d.xml", i))
			content := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>OpenBrief limit fixture %d</title>
<item><title>OpenBrief limit story %d</title><link>https://fixture.example/limit-%d</link><guid>limit-guid-%d</guid><pubDate>Thu, 23 Apr 2026 01:0%d:00 GMT</pubDate></item>
</channel></rss>`, i, i, i, i, i)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return scenario{}, err
			}
			limitFeedURLs = append(limitFeedURLs, (&url.URL{Scheme: "file", Path: path}).String())
		}
	}
	releases := `[{"tag_name":"v1.2.3","name":"OpenBrief fixture release","html_url":"https://fixture.example/releases/tag/v1.2.3","published_at":"2026-04-23T01:00:00Z","draft":false,"prerelease":false}]`
	if needsReleases {
		if err := os.WriteFile(releasePath, []byte(releases), 0o644); err != nil {
			return scenario{}, err
		}
	}
	replacementFeed := (&url.URL{Scheme: "file", Path: feedPath}).String()
	replacementReleases := (&url.URL{Scheme: "file", Path: releasePath}).String()
	rewrite := func(prompt string) string {
		prompt = strings.ReplaceAll(prompt, "https://github.blog/feed/", replacementFeed)
		prompt = strings.ReplaceAll(prompt, "named github.blog", "named fixture.example")
		prompt = strings.ReplaceAll(prompt, "repository openai/codex with key", "repository openai/codex using source URL "+replacementReleases+" with key")
		for i, value := range limitFeedURLs {
			prompt = strings.ReplaceAll(prompt, fmt.Sprintf("https://example.com/openbrief-limit-%d.xml", i+1), value)
		}
		return prompt
	}
	for i := range current.Turns {
		current.Turns[i].Prompt = rewrite(current.Turns[i].Prompt)
	}
	return current, nil
}

func scenarioContains(current scenario, value string) bool {
	for _, turn := range current.Turns {
		if strings.Contains(turn.Prompt, value) {
			return true
		}
	}
	return false
}
