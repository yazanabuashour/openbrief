package runner

import (
	"context"
	"fmt"

	"github.com/yazanabuashour/openbrief/internal/runclient"
)

const (
	ConfigActionInit                  = "init"
	ConfigActionInspectConfig         = "inspect_config"
	ConfigActionReplaceSources        = "replace_sources"
	ConfigActionUpsertSource          = "upsert_source"
	ConfigActionDeleteSource          = "delete_source"
	ConfigActionReplaceOutletPolicies = "replace_outlet_policies"
)

func RunConfigTask(ctx context.Context, cfg runclient.Config, request ConfigTaskRequest) (ConfigTaskResult, error) {
	rt, err := runclient.Open(ctx, cfg)
	if err != nil {
		return ConfigTaskResult{}, err
	}
	defer func() {
		_ = rt.Close()
	}()
	paths := rt.Paths()
	store := rt.Store()

	switch request.Action {
	case ConfigActionInit:
		runtimeConfig, err := store.RuntimeConfig(ctx)
		if err != nil {
			return ConfigTaskResult{}, err
		}
		return ConfigTaskResult{
			Paths:         paths,
			RuntimeConfig: runtimeConfig,
			Summary:       "initialized OpenBrief database",
		}, nil
	case ConfigActionInspectConfig:
		return inspectConfig(ctx, paths, store)
	case ConfigActionReplaceSources:
		sources, err := store.ReplaceSources(ctx, request.Sources)
		if err != nil {
			return rejectedConfig(paths, err.Error()), nil
		}
		return ConfigTaskResult{
			Paths:   paths,
			Sources: sources,
			Summary: fmt.Sprintf("stored %d sources", len(sources)),
		}, nil
	case ConfigActionUpsertSource:
		source, err := store.UpsertSource(ctx, request.Source)
		if err != nil {
			return rejectedConfig(paths, err.Error()), nil
		}
		return ConfigTaskResult{
			Paths:   paths,
			Sources: []Source{source},
			Summary: fmt.Sprintf("stored source %s", source.Key),
		}, nil
	case ConfigActionDeleteSource:
		if err := store.DeleteSource(ctx, request.Key); err != nil {
			return rejectedConfig(paths, err.Error()), nil
		}
		sources, err := store.ListSources(ctx, false)
		if err != nil {
			return ConfigTaskResult{}, err
		}
		return ConfigTaskResult{
			Paths:   paths,
			Sources: sources,
			Summary: fmt.Sprintf("deleted source %s", request.Key),
		}, nil
	case ConfigActionReplaceOutletPolicies:
		outlets, err := store.ReplaceOutletPolicies(ctx, request.Outlets)
		if err != nil {
			return rejectedConfig(paths, err.Error()), nil
		}
		return ConfigTaskResult{
			Paths:   paths,
			Outlets: outlets,
			Summary: fmt.Sprintf("stored %d outlet policies", len(outlets)),
		}, nil
	default:
		return rejectedConfig(paths, fmt.Sprintf("unsupported config action %q", request.Action)), nil
	}
}

type configStore interface {
	RuntimeConfig(context.Context) (map[string]string, error)
	ListSources(context.Context, bool) ([]Source, error)
	ListOutletPolicies(context.Context) ([]OutletPolicy, error)
}

func inspectConfig(ctx context.Context, paths Paths, store configStore) (ConfigTaskResult, error) {
	runtimeConfig, err := store.RuntimeConfig(ctx)
	if err != nil {
		return ConfigTaskResult{}, err
	}
	sources, err := store.ListSources(ctx, false)
	if err != nil {
		return ConfigTaskResult{}, err
	}
	outlets, err := store.ListOutletPolicies(ctx)
	if err != nil {
		return ConfigTaskResult{}, err
	}
	return ConfigTaskResult{
		Paths:         paths,
		RuntimeConfig: runtimeConfig,
		Sources:       sources,
		Outlets:       outlets,
		Summary:       fmt.Sprintf("configured sources=%d outlet_policies=%d", len(sources), len(outlets)),
	}, nil
}

func rejectedConfig(paths Paths, reason string) ConfigTaskResult {
	return ConfigTaskResult{
		Rejected:        true,
		RejectionReason: reason,
		Paths:           paths,
		Summary:         reason,
	}
}
