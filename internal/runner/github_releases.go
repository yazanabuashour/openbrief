package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (f fetcher) fetchGitHubReleases(ctx context.Context, source Source) ([]fetchedItem, error) {
	endpoint := strings.TrimSpace(source.URL)
	if endpoint == "" {
		endpoint = "https://api.github.com/repos/" + source.Repo + "/releases?per_page=10"
	}
	body, err := f.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("parse GitHub releases JSON: %w", err)
	}
	var items []fetchedItem
	for _, release := range releases {
		if release.Draft || release.Prerelease || strings.TrimSpace(release.TagName) == "" {
			continue
		}
		title := strings.TrimSpace(release.Name)
		if title == "" {
			title = release.TagName
		}
		if source.Repo != "" {
			title = source.Repo + " " + title
		}
		releaseURL := release.HTMLURL
		if releaseURL == "" && source.Repo != "" {
			releaseURL = "https://github.com/" + source.Repo + "/releases/tag/" + release.TagName
		}
		items = append(items, fetchedItem{
			Title:       title,
			URL:         releaseURL,
			PublishedAt: release.PublishedAt,
			Identity:    release.TagName,
		})
	}
	return items, nil
}
