package domain

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const (
	SourceKindRSS           = "rss"
	SourceKindAtom          = "atom"
	SourceKindGitHubRelease = "github_release"

	ThresholdAlways = "always"
	ThresholdMedium = "medium"
	ThresholdHigh   = "high"
	ThresholdAudit  = "audit"

	URLCanonicalizationNone               = "none"
	URLCanonicalizationFeedBurnerRedirect = "feedburner_redirect"
	URLCanonicalizationGoogleNewsArticle  = "google_news_article_url"

	OutletExtractionNone        = "none"
	OutletExtractionTitleSuffix = "title_suffix"
	OutletExtractionURLHost     = "url_host"
	OutletExtractionRSSSource   = "rss_source"

	EvalAllowFileURLsEnv = "OPENBRIEF_EVAL_ALLOW_FILE_URLS"
)

var (
	sourceKeyPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	gitHubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

type Source struct {
	Key                 string `json:"key"`
	Label               string `json:"label"`
	Kind                string `json:"kind"`
	URL                 string `json:"url,omitempty"`
	Repo                string `json:"repo,omitempty"`
	Section             string `json:"section"`
	Threshold           string `json:"threshold"`
	Enabled             bool   `json:"enabled"`
	URLCanonicalization string `json:"url_canonicalization,omitempty"`
	OutletExtraction    string `json:"outlet_extraction,omitempty"`
	DedupGroup          string `json:"dedup_group,omitempty"`
	PriorityRank        int    `json:"priority_rank,omitempty"`
	AlwaysReport        bool   `json:"always_report,omitempty"`
}

func NormalizeSources(sources []Source) ([]Source, error) {
	seen := map[string]struct{}{}
	normalized := make([]Source, 0, len(sources))
	for _, source := range sources {
		next, err := NormalizeSource(source)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[next.Key]; ok {
			return nil, fmt.Errorf("duplicate source key %q", next.Key)
		}
		seen[next.Key] = struct{}{}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func NormalizeSource(source Source) (Source, error) {
	source.Key = strings.TrimSpace(strings.ToLower(source.Key))
	source.Label = strings.TrimSpace(source.Label)
	source.Kind = strings.TrimSpace(strings.ToLower(source.Kind))
	source.URL = strings.TrimSpace(source.URL)
	source.Repo = strings.TrimSpace(source.Repo)
	source.Section = strings.TrimSpace(strings.ToLower(source.Section))
	source.Threshold = strings.TrimSpace(strings.ToLower(source.Threshold))
	source.URLCanonicalization = strings.TrimSpace(strings.ToLower(source.URLCanonicalization))
	source.OutletExtraction = strings.TrimSpace(strings.ToLower(source.OutletExtraction))
	source.DedupGroup = strings.TrimSpace(strings.ToLower(source.DedupGroup))
	if source.Key == "" || !sourceKeyPattern.MatchString(source.Key) {
		return Source{}, errors.New("source key must be lowercase letters, numbers, dot, underscore, or hyphen")
	}
	if source.Label == "" {
		return Source{}, fmt.Errorf("source %q label is required", source.Key)
	}
	if source.Section == "" {
		return Source{}, fmt.Errorf("source %q section is required", source.Key)
	}
	if source.Threshold == "" {
		source.Threshold = ThresholdMedium
	}
	if source.URLCanonicalization == "" {
		source.URLCanonicalization = URLCanonicalizationNone
	}
	if source.OutletExtraction == "" {
		source.OutletExtraction = OutletExtractionNone
	}
	switch source.Threshold {
	case ThresholdAlways, ThresholdMedium, ThresholdHigh, ThresholdAudit:
	default:
		return Source{}, fmt.Errorf("source %q threshold must be always, medium, high, or audit", source.Key)
	}
	switch source.URLCanonicalization {
	case URLCanonicalizationNone, URLCanonicalizationFeedBurnerRedirect, URLCanonicalizationGoogleNewsArticle:
	default:
		return Source{}, fmt.Errorf("source %q url_canonicalization must be none, feedburner_redirect, or google_news_article_url", source.Key)
	}
	switch source.OutletExtraction {
	case OutletExtractionNone, OutletExtractionTitleSuffix, OutletExtractionURLHost, OutletExtractionRSSSource:
	default:
		return Source{}, fmt.Errorf("source %q outlet_extraction must be none, title_suffix, url_host, or rss_source", source.Key)
	}
	switch source.Kind {
	case SourceKindRSS, SourceKindAtom:
		if err := validateFetchURL(source.URL); err != nil {
			return Source{}, fmt.Errorf("source %q url: %w", source.Key, err)
		}
	case SourceKindGitHubRelease:
		if source.Repo == "" && source.URL == "" {
			return Source{}, fmt.Errorf("source %q repo or url is required for github_release", source.Key)
		}
		if source.Repo != "" && !gitHubRepoPattern.MatchString(source.Repo) {
			return Source{}, fmt.Errorf("source %q repo must be owner/name", source.Key)
		}
		if source.URL != "" {
			if err := validateFetchURL(source.URL); err != nil {
				return Source{}, fmt.Errorf("source %q url: %w", source.Key, err)
			}
		}
	default:
		return Source{}, fmt.Errorf("source %q kind must be rss, atom, or github_release", source.Key)
	}
	return source, nil
}

func validateFetchURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme == "file" && os.Getenv(EvalAllowFileURLsEnv) == "1" {
		if parsed.Path == "" {
			return errors.New("file URL must include a path")
		}
		return nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}
