# POC: Provider Strategy

## Status

Superseded by generic feed processing in the production runner.

## Decision

The `openbrief` runner supports:

- RSS feeds
- Atom feeds
- GitHub releases via the public GitHub releases API
- optional URL canonicalization on feed items
- optional outlet extraction on feed items
- outlet policy suppression and audit
- same-run topic deduplication
- recent-delivery topic suppression

## Rationale

Feed providers must not become source kinds unless they have a genuinely
different fetch interface. Google News topic/search feeds, blogs, newsletters,
and similar inputs are ordinary RSS/Atom sources configured by the operator.

Provider-specific behavior is modeled as optional post-fetch processing. For
example, Google News article URL resolution is a URL canonicalization strategy
on a normal RSS source, not a `google_news` source type. The repository still
ships no personal feed inventory, outlet policy evidence, delivery history, or
local state.
