# readlist-tagger: Readeck support — design

## Intent

Gabriel moved from Wallabag to Readeck (srv-1, 2026-11-21) and reads on the Kobo through the
KOReader plugin `iceyear/readeck.koplugin`. The tagger's job doesn't change: put theme tags on every
new unread article so he can filter by theme on the Kobo, offline, and drop non-reading material
(`hors-liseuse`) at sync. Only the service it talks to changes.

The repo was renamed `wallabag-tagger` → `readlist-tagger` (GitHub redirects the old URL). Wallabag
support stays for anyone else who uses it. It is documented as working but no longer used day to
day by its author.

Everything in `2026-11-19-wallabag-tagger-design.md` still holds (seed tags, new-tag rules, run
loop, failure counter, Claude invocation, observability, anti-goals) unless this document says
otherwise.

## How the tags reach the Kobo with Readeck (verified in code)

- The plugin writes each bookmark's `labels` into KOReader's custom metadata keywords on download
  (`readeck/core/metadata.lua`, `Metadata.article_keywords`), so they can be filtered by keyword
  like the Wallabag EPUB subjects were.
- "Tags to ignore" in the plugin settings excludes `hors-liseuse` at sync.
- The Wallabag import carried the existing tags into Readeck labels, so the seed tags and older
  model-created tags are already there.

## Goals

- `readeck-tagger` behaves exactly like `wallabag-tagger` does today, against Readeck 0.22+.
- `wallabag-tagger` keeps working unchanged: same binary name, same environment variables.
- One shared domain: seed tags, prompts, validation and run loop are written once.

## Non-goals

- No runtime backend choice (no `BACKEND=` variable): each backend is its own binary.
- No new features (no re-tagging, no tag clean-up, no config file for seed tags).
- No migration of `failures.json` between backends: the new service starts with an empty one.

## Anti-goals

The ones in the original design, plus:

- Must not tag a Readeck bookmark whose content hasn't finished extracting. Tagging on an empty
  body would mark it done (it has a label) with a guess made on the title alone.
- Must not break the Wallabag binary or change its interface.

## Approaches considered

- **A. Two binaries sharing the domain (picked).** `cmd/wallabag-tagger` (unchanged) and
  `cmd/readeck-tagger`, each reading its own environment and building its adapter. This follows the
  repo's layout rule (entry points wire adapters, no selection logic) and leaves Wallabag users
  untouched. The cost is two near-identical `main.go` files.
- **B. One binary with the backend chosen in config.** One name, but it needs a switch, cross-
  validation of mixed variables, and Wallabag users would have to change binary and add a
  variable. Rejected.
- **C. Separate repos.** Duplicates the domain and prompts. Rejected.

## Changes

### Repo-wide

- Go module path `github.com/Crocmagnon/wallabag-tagger` → `github.com/Crocmagnon/readlist-tagger`.
- README, CLAUDE.md and flake description talk about read-it-later services in general. The README
  has one section per backend (variables, setup), and the Wallabag section carries an
  "unmaintained: works, no longer used by the author" note.

### Domain (`internal/domain/tagging`)

- `EntryID` becomes `string`. Readeck ids are strings (`XChJDtQ4cN12WJs2VcgcmX`). Wallabag ids are
  formatted as decimal strings by its adapter. The `failures.json` format (`{"<id>": count}`) is
  unchanged on disk, so an existing Wallabag state file still loads.
- Wording only: "Wallabag" in comments and in the `Tag` doc becomes generic. `Existing(wallabagTags)`
  is renamed to a neutral parameter name.
- No behaviour change. Skipping bookmarks that are still loading is the Readeck adapter's job
  (below), so `Run` stays as is.

### New adapter `internal/adapters/readeck`

Implements `tagging.Articles` over the Readeck API (`/api`, docs in Readeck's
`docs/api/bookmarks/routes-bookmarks.yaml`), authenticated with an API token
(`Authorization: Bearer <token>`).

- `Tags`: `GET /api/bookmarks/labels`, returns every label name.
- `UnreadEntries`: `GET /api/bookmarks?is_archived=false&has_labels=false&is_loaded=true`, paginated
  with `limit`/`offset` until a page comes back short (or using `Total-Count`). `Entry.Content`
  depends on the bookmark's `has_article`:
  - `true`: `GET /api/bookmarks/{id}/article` gives the HTML body. If that call fails, the error
    fails `UnreadEntries` and therefore the run. With an article present, a failure there is
    transient (Readeck down or restarting), and the next minute retries it.
  - `false`: the body is not fetched. `Entry.Content` is the bookmark's `description` (may be
    empty), and the model judges on title, site, URL and that description. This is required, not
    an optimisation: in 0.22.3 the `/article` route panics into a 500 when the bookmark has no
    archive (`api_bookmarks.go`, `OpenContainer` returns nil and `ListResources` is called on
    it). Fetching the body anyway would make one broken bookmark fail every run forever, since
    the failure counter only sees errors raised inside `Run`. *(Designer's call, raised by the
    builder check.)*
  `Entry.Domain` is the bookmark's `site`. `Entry.Tags` is the bookmark's `labels`. With
  `has_labels=false` that list is always empty, and filling it anyway keeps `Run`'s own check
  meaningful.
  - `is_loaded=true` excludes bookmarks whose extraction is still running. They are picked up on a
    later run (the timer runs every minute).
  - Bookmarks with `has_errors=true` are not excluded: extraction finished, possibly with no
    article (`has_article=false`, handled above). The model judges on title, site and URL. That is what Wallabag did with entries it
    couldn't fetch, and such pages are usually `hors-liseuse`.
- `AddTags`: `PATCH /api/bookmarks/{id}` with `{"add_labels": [...]}`. It adds labels and never
  removes any, as the anti-goals require.

### New entry point `cmd/readeck-tagger`

Same shape as `cmd/wallabag-tagger`: required variables `READECK_URL`, `READECK_TOKEN`, plus the
shared `CLAUDE_CODE_OAUTH_TOKEN` (read by `claude`), `STATE_DIRECTORY` and `CLAUDE_BIN`. Same exit
codes (0 / 1 / 2), same logger, same Claude settings. No login step.

### Flake

Two packages, `wallabag-tagger` and `readeck-tagger`, and `default = readeck-tagger`.

## Deployment (nixos-config, separate PR after this one is merged)

- Flake input renamed `readlist-tagger` (`github:Crocmagnon/readlist-tagger`), overlay exposing
  `readeck-tagger`.
- `hosts/srv-1/services/wallabag-tagger.nix` replaced by `readeck-tagger.nix`. Same timer,
  healthchecks renamed `readeck-tagger`, `READECK_URL = "http://localhost:8006"`,
  `StateDirectory = "readeck-tagger"`, after `readeck.service`.
- Secret `readeck-tagger-env.age`: `READECK_TOKEN` and `CLAUDE_CODE_OAUTH_TOKEN` (the existing
  token can be reused). Gabriel creates it (Claude Code is denied `agenix`). He also creates the
  Readeck API token in Readeck → Profile → API tokens, with bookmark read and write rights.
- The Wallabag service, its tagger, secret and DNS go away later, with Wallabag itself.
- Manual step on the Kobo: plugin "Tags to ignore" → `hors-liseuse`.

## Left to the builder

- Page size for the bookmark list.
- Whether the adapter fetches article bodies lazily or all up front (a handful per run in steady
  state).
- Tests: the Readeck adapter against an `httptest` server (filters sent, pagination, body fetch
  only when `has_article`, description fallback, `add_labels` payload), on the model of the Wallabag adapter's tests; domain tests updated for
  string ids.
