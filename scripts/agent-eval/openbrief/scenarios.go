package main

const productionRunnerOnlyInstruction = "Use only the openbrief runner JSON interface. Do not inspect repo files, source files, skill files, binaries, SQLite, environment variables, or run openbrief --help. Do not search for instructions."

type scenarioTurn struct {
	Prompt string
}

type scenario struct {
	ID    string
	Turns []scenarioTurn
}

var scenarios = []scenario{
	{
		ID: "empty-config-rejects-run-brief",
		Turns: []scenarioTurn{{
			Prompt: "Run an OpenBrief brief from a fresh empty configuration and report the production runner result.",
		}},
	},
	{
		ID: "rss-source-first-run-candidate",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Then run an OpenBrief brief and report the JSON-derived brief.",
		}},
	},
	{
		ID: "github-release-source-must-include",
		Turns: []scenarioTurn{{
			Prompt: "Configure a GitHub release source for repository openai/codex with key codex-releases, section releases, and threshold always. Then run an OpenBrief brief and report the JSON-derived brief.",
		}},
	},
	{
		ID: "repeat-run-no-new-items",
		Turns: []scenarioTurn{
			{Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Run an OpenBrief brief and record any delivered message when required."},
			{Prompt: "Run OpenBrief again without changing configuration and report the production runner result."},
		},
	},
	{
		ID: "record-delivery-suppresses-repeats",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Run an OpenBrief brief, record the delivered message when required, then run the brief again and report whether repeats were suppressed.",
		}},
	},
	{
		ID: "rss-source-generic-processing-fields",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, threshold medium, url_canonicalization none, outlet_extraction url_host, dedup_group news, and priority_rank 10. Then run OpenBrief and report only the JSON-derived result.",
		}},
	},
	{
		ID: "outlet-policy-watch-audit",
		Turns: []scenarioTurn{{
			Prompt: "Configure an outlet policy named github.blog with policy watch and enabled true. Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, threshold medium, and outlet_extraction url_host. Run OpenBrief and report whether the JSON result includes a policy audit while still allowing candidates.",
		}},
	},
	{
		ID: "configured-max-delivery-items",
		Turns: []scenarioTurn{{
			Prompt: "Configure OpenBrief max_delivery_items to 2 through openbrief config. Configure RSS sources with keys limit-one, limit-two, and limit-three for https://example.com/openbrief-limit-1.xml, https://example.com/openbrief-limit-2.xml, and https://example.com/openbrief-limit-3.xml, each with section technology and threshold medium. Run an OpenBrief brief, deliver the brief according to max_delivery_items, and record the exact delivered message when required. Report only the delivered brief.",
		}},
	},
	{
		ID: "feed-failure-health-footnote",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source with key broken-feed, label Broken Feed, URL https://127.0.0.1:1/no-feed.xml, section technology, and threshold medium. Run an OpenBrief brief and report the health footnote from the JSON result.",
		}},
	},
	{
		ID: "feed-recovery-resolves-warning",
		Turns: []scenarioTurn{
			{Prompt: "Configure an RSS source with key changing-feed, label Changing Feed, URL https://127.0.0.1:1/no-feed.xml, section technology, and threshold medium. Run an OpenBrief brief and report the JSON result."},
			{Prompt: "Replace the changing-feed source URL with https://github.blog/feed/. Run OpenBrief again and report the JSON-derived result."},
		},
	},
	{
		ID: "invalid-source-config-rejects",
		Turns: []scenarioTurn{{
			Prompt: "Try to configure an OpenBrief source with an invalid key Bad/Key by piping one upsert_source JSON request to openbrief config. Report the production runner rejection. Do not inspect repo files, skill files, binaries, SQLite, environment variables, or run openbrief --help.",
		}},
	},
	{
		ID: "routine-agent-hygiene",
		Turns: []scenarioTurn{{
			Prompt: "Run a normal OpenBrief configuration inspection by piping exactly {\"action\":\"inspect_config\"} to openbrief config. For this routine production task, do not inspect SQLite, source files, skill files, repo files, binaries, or environment variables. Do not run openbrief --help or search for instructions.",
		}},
	},
}
