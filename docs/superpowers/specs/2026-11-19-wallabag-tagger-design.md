# wallabag-tagger — design

## Intent

Gabriel reads his self-hosted Wallabag on a Kobo running KoReader, through KoReader's built-in
Wallabag plugin. With 100+ unread articles, the Kobo shows a flat list of titles with no way to
pick what fits his mood of the moment (lately AI and work methods rather than Go internals;
sometimes databases), and the list is polluted by saved pages that are not reading material at
all (GitHub repos, videos, tool homepages).

wallabag-tagger puts **theme tags** on every unread article, automatically, so that he can filter
by theme **on the Kobo, offline**.

## How the tags reach the Kobo (verified, no fork needed)

- Wallabag's EPUB export writes each tag as an EPUB `subject`
  (`src/Helper/EntriesExport.php`); all tags of an article come through (tested on device).
- KoReader shows subjects as "Keywords" and can filter a collection by keyword offline
  (`frontend/apps/filemanager/filemanagercollection.lua`). Gabriel has a "Wallabag" collection
  connected to the plugin's download folder.
- The plugin's "Do not download articles with tags" setting excludes `hors-liseuse` at sync.
- KoReader caches book metadata: tags added to an already-downloaded article only show after
  "Refresh cached book information" or a re-download. New articles are fine since the tagger runs
  within a minute of the article being saved, before any sync.

## Goals

- Every unread article gets one or more tags within about a minute of being saved, chosen from
  the existing tags first; the model may create a new tag when none fits.
- Articles that are not reading material get `hors-liseuse`.
- Runs unattended on srv-1, at no extra cost beyond Gabriel's Claude subscription.
- Observable from `journalctl`: what it tagged, what it created, what failed, what it cost.

## Non-goals

- Archived (read) articles are never tagged.
- No KoReader fork or patch.
- No automatic merging or clean-up of tags: Gabriel prunes by hand, helped by the logs.

## Anti-goals

- Must not touch an article's content, title, read/starred state, or remove any tag.
- Must not re-tag in a loop, nor spend Claude calls on an article that is already tagged.
- Must not let tags proliferate: a Kobo keyword list of dozens of near-duplicates defeats the
  purpose.
- Must not force an article into an ill-fitting existing tag: a wrong tag pollutes that theme's
  filter, which is worse than a tag used once.
- Article content is untrusted input: a page must not be able to make the tagger do anything
  beyond picking or naming tags.

## Seed tags

The 13 tags below are defined in code, with their description, and are always offered to the
model. They were derived once, by hand, from the existing 1,675 articles. Lowercase, no accents
(legible and sortable on the Kobo). Several tags per article are allowed.

| Tag | Scope |
|---|---|
| `ia` | LLMs, agents, AI, and criticism of the AI hype |
| `go` | The Go language, its internals, concurrency, testing in Go |
| `bdd` | Postgres, SQL, data storage |
| `methodes` | Management, leadership, feedback, design docs/RFCs, estimation, agile, career |
| `ingenierie` | Language-agnostic software craft: architecture, testing, distributed systems, tech choices |
| `langages` | Other languages: Rust, Python/Django, Zig, C, JS/CSS |
| `systeme` | Linux, Nix, git, terminal, filesystems, self-hosting |
| `reseau` | The Internet, DNS, email, network governance |
| `securite` | Security, cryptography, privacy |
| `societe` | Tech and society: digital sovereignty, enshittification, environmental footprint |
| `histoire` | History and culture of computing |
| `hors-tech` | Personal finance, cooking, health, fiction, literature |
| `hors-liseuse` | Not long-form reading material: code repositories, videos, podcasts, tool or product homepages, tweets |

`hors-liseuse` is decided by the model, not by Wallabag domain rules: it also catches tool homepages
and media a domain list would miss. An article may carry `hors-liseuse` alone.

## Existing and new tags

- **Existing tags** = the seed tags plus every other tag currently in Wallabag
  (`GET /api/tags.json`, read at each run). Tags created earlier by the model (or by hand) are
  offered by name only, without a description.
- The model picks as many **existing** tags as genuinely fit.
- It may also propose **at most one new tag** per article, only when no existing tag fits a
  significant theme of the article. A new tag may come alone or next to existing ones; there is
  no obligation to also pick an existing tag.
- At least one tag in total per article.
- A new tag must match `^[a-z0-9-]{2,20}$` (checked in Go, not only in the schema) and must not
  duplicate an existing tag.
- The prompt says to prefer existing tags and to name new ones as broad, reusable themes, not
  article-specific labels.

## Behaviour

Every minute (systemd timer on srv-1):

1. Get an OAuth token from Wallabag (password grant), read the tag list, list **unread** entries.
2. Keep entries that carry **no tag at all**.
3. For each one, call `claude -p` with the title, domain, URL and the beginning of the article
   text (HTML stripped), plus the existing tags (seed tags with descriptions). Output is
   structured JSON constrained by `--json-schema`, built per run: `tags` (array, items an enum of
   the existing tags) and an optional `new_tag` (string, pattern above).
4. Validate the answer in Go (enum, pattern, at most one new tag, at least one tag in total);
   then add the tags to the entry (`POST /api/entries/{id}/tags.json`). The new tag is created
   by that call. The next article in the same run sees it as existing.
5. If an entry fails (invalid output, API error on that entry), go on with the others, then exit
   non-zero so the healthcheck goes red. The failed entry has no tag, so the next run retries it.
6. If the `claude` CLI itself fails (auth, usage limit, network), stop the run at once instead of
   trying the remaining entries; exit non-zero.

"Has at least one tag" is the only marker of "done" (possible because all old tags were deleted
and Gabriel no longer tags by hand). The one piece of state is a small failure counter per entry
id, in a file under the service's state directory (which exists anyway as `claude`'s `$HOME`):
after **3** failed attempts an entry is skipped, so a poisoned article does not cost a Claude call
every minute. While any entry is skipped this way, the run exits non-zero (healthcheck stays red,
the log names the entry ids). Deleting the file resets it. *(Designer's call: a Wallabag marker
tag would show up as a keyword on the Kobo.)*

Removing every tag from an article by hand makes it eligible again: it will be re-tagged within a
minute. To correct a wrong tag, replace it with the right one. There is no "no theme" opt-out.
*(Designer's call: rare case, not worth a marker.)*

## Observability

Logs go to stdout/stderr (journald), plain text, one event per line, greppable:

- Each tagged article: entry id, title, tags applied, duration, and the call's cost/usage as
  reported by `claude -p --output-format json`. When a tag is created, the line carries a fixed
  marker `NEW TAG <name>` so `journalctl -u wallabag-tagger | grep "NEW TAG"` lists every
  creation.
- Each failure: entry id, cause, and the `claude` stderr (truncated).
- Entries skipped after 3 failures: their ids, on every run.
- A summary per run (examined, tagged, failed, skipped, total cost), **only when there was
  work**: an idle run writes nothing, so the journal is not one line per minute. The healthcheck
  pings show the service is alive.

The first run tags the 135 currently unread articles (old tags were already deleted by hand on
2026-11-19).

## Claude invocation

- Claude Code CLI, print mode, authenticated with `CLAUDE_CODE_OAUTH_TOKEN` (generated with
  `claude setup-token`, valid one year — the same mechanism as the `auto-fix` workflow in
  nixos-config). This uses Gabriel's subscription; no API key.
- No tools at all: `--tools ""`. Combined with the enum-constrained output, a malicious page can
  at worst get a wrong tag.
- `claude` needs a writable `$HOME` for its config: the service's `StateDirectory`.
- First thing to verify when building: the nixpkgs-unstable `claude-code` accepts `--json-schema`
  and `--tools ""` in print mode and authenticates headless with `CLAUDE_CODE_OAUTH_TOKEN`.
- Every call counts against Gabriel's interactive subscription quota; the first run (135 calls)
  is the only big one.

## Deployment

- **This repo**: Go module `github.com/Crocmagnon/wallabag-tagger`, Unlicense, default branch
  `main`. A `flake.nix` exposing `packages.<system>.wallabag-tagger` via `buildGoModule`, on the
  model of `Crocmagnon/downloader-go`'s flake.
- **nixos-config**: a flake input plus overlay (like `downloader` /
  `overlays/playwright-downloader.nix`), and `hosts/srv-1/services/wallabag-tagger.nix` on the
  model of `paperless-hass-sync.nix`: oneshot service + minutely timer (`Persistent = true`),
  `healthchecks@wallabag-tagger:{start,success,failure}`, `restartIfChanged = false`.
- Wallabag is reached at `http://localhost:8002` (the container's port on srv-1), not through
  Caddy.
- Secrets: agenix `wallabag-tagger-env.age` (one rule in `hosts/srv-1/age/agenix-rules.nix`)
  holding `WALLABAG_URL`, `WALLABAG_CLIENT_ID`, `WALLABAG_CLIENT_SECRET`, `WALLABAG_USERNAME`,
  `WALLABAG_PASSWORD`, `CLAUDE_CODE_OAUTH_TOKEN`. Gabriel creates the encrypted file himself
  (Claude Code is denied `agenix`).
- `claude-code` comes from nixpkgs-unstable: add it to `overlays/unstable-packages.nix`.

## Manual steps after deploy (Gabriel)

- KoReader Wallabag plugin: "Do not download articles with tags" → `hors-liseuse`.
- Delete the local Wallabag folder on the Kobo and sync again, so already-downloaded articles come
  back with their tags.

## Left to the builder

- How much article text to send (enough to judge the theme; a few thousand characters).
- Prompt wording, model flag, CLI flags beyond the ones above.
- Go package layout, HTTP client, tests (at least: untagged-entry filtering, output validation (enum, new-tag pattern and limit),
  prompt building; Wallabag and `claude` behind small interfaces or fakes).
- Running Claude calls sequentially is fine (a handful of new articles a week; 135 once).
