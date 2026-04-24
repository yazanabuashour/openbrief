package runner

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

type fetchedItem struct {
	Title       string
	URL         string
	PublishedAt string
	Identity    string
}

type fetcher struct {
	client *http.Client
}

func newFetcher() fetcher {
	return fetcher{client: &http.Client{Timeout: 20 * time.Second}}
}

func (f fetcher) Fetch(ctx context.Context, source Source) ([]fetchedItem, error) {
	switch source.Kind {
	case sqlite.SourceKindRSS, sqlite.SourceKindAtom:
		return f.fetchFeed(ctx, source)
	case sqlite.SourceKindGitHubRelease:
		return f.fetchGitHubReleases(ctx, source)
	default:
		return nil, fmt.Errorf("unsupported source kind %q", source.Kind)
	}
}

func (f fetcher) fetchFeed(ctx context.Context, source Source) ([]fetchedItem, error) {
	body, err := f.get(ctx, source.URL)
	if err != nil {
		return nil, err
	}
	items, err := parseFeed(body)
	if err != nil {
		return nil, err
	}
	return items, nil
}

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

func (f fetcher) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "openbrief/0")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, endpoint)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Links   []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func parseFeed(body []byte) ([]fetchedItem, error) {
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && rss.XMLName.Local == "rss" {
		items := make([]fetchedItem, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			title := cleanText(item.Title)
			link := strings.TrimSpace(item.Link)
			identity := strings.TrimSpace(item.GUID)
			if identity == "" {
				identity = link
			}
			if identity == "" {
				identity = title
			}
			if title != "" {
				items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(item.PubDate), Identity: identity})
			}
		}
		return items, nil
	}
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err != nil {
		return nil, fmt.Errorf("parse feed XML: %w", err)
	}
	if atom.XMLName.Local != "feed" {
		return nil, fmt.Errorf("parse feed XML: unsupported feed root %q", atom.XMLName.Local)
	}
	items := make([]fetchedItem, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		title := cleanText(entry.Title)
		link := atomEntryLink(entry)
		identity := strings.TrimSpace(entry.ID)
		if identity == "" {
			identity = link
		}
		if identity == "" {
			identity = title
		}
		if title != "" {
			items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(entry.Updated), Identity: identity})
		}
	}
	return items, nil
}

func atomEntryLink(entry atomEntry) string {
	if len(entry.Links) == 0 {
		return ""
	}
	for _, link := range entry.Links {
		if link.Rel == "" || link.Rel == "alternate" {
			return strings.TrimSpace(link.Href)
		}
	}
	return strings.TrimSpace(entry.Links[0].Href)
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
