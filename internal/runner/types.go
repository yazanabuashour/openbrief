package runner

import (
	"time"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

type Paths = runclient.Paths
type Source = sqlite.Source
type OutletPolicy = sqlite.OutletPolicy

type ConfigTaskRequest struct {
	Action  string         `json:"action"`
	Source  Source         `json:"source,omitempty"`
	Sources []Source       `json:"sources,omitempty"`
	Key     string         `json:"key,omitempty"`
	Outlets []OutletPolicy `json:"outlets,omitempty"`
}

type ConfigTaskResult struct {
	Rejected        bool              `json:"rejected"`
	RejectionReason string            `json:"rejection_reason,omitempty"`
	Paths           Paths             `json:"paths"`
	RuntimeConfig   map[string]string `json:"runtime_config,omitempty"`
	Sources         []Source          `json:"sources,omitempty"`
	Outlets         []OutletPolicy    `json:"outlets,omitempty"`
	Summary         string            `json:"summary"`
}

type BriefTaskRequest struct {
	Action  string `json:"action"`
	DryRun  bool   `json:"dry_run,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Message string `json:"message,omitempty"`
}

type BriefTaskResult struct {
	Rejected        bool               `json:"rejected"`
	RejectionReason string             `json:"rejection_reason,omitempty"`
	Paths           Paths              `json:"paths"`
	RunID           string             `json:"run_id,omitempty"`
	MustInclude     []BriefItem        `json:"must_include,omitempty"`
	Candidates      []BriefItem        `json:"candidates,omitempty"`
	RecentSent      []SentItem         `json:"recent_sent,omitempty"`
	Suppressed      []SuppressedItem   `json:"suppressed,omitempty"`
	FetchStatus     []FetchStatus      `json:"fetch_status,omitempty"`
	HealthFootnote  string             `json:"health_footnote,omitempty"`
	HealthDelta     sqlite.HealthDelta `json:"health_delta,omitempty"`
	SentItems       []SentItem         `json:"sent_items,omitempty"`
	Summary         string             `json:"summary"`
}

type BriefItem struct {
	SourceKey   string `json:"source_key"`
	SourceLabel string `json:"source_label"`
	Kind        string `json:"kind"`
	Section     string `json:"section"`
	Threshold   string `json:"threshold"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at,omitempty"`
}

type SuppressedItem struct {
	SourceKey string `json:"source_key"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Reason    string `json:"reason"`
}

type FetchStatus struct {
	SourceKey string `json:"source_key"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	Items     int    `json:"items"`
	NewItems  int    `json:"new_items"`
}

type SentItem struct {
	Title  string    `json:"title"`
	URL    string    `json:"url"`
	SentAt time.Time `json:"sent_at"`
}
