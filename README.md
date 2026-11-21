# readlist-tagger

Tags every unread, untagged article of a read-it-later service by theme, with Claude
(`claude -p`), so the articles can be filtered by theme offline on a KOReader e-reader.

Two binaries share the same rules:

- `readeck-tagger` for [Readeck](https://readeck.org) (maintained, used daily by the author);
- `wallabag-tagger` for [Wallabag](https://wallabag.org) (unmaintained, see below).

Run one every minute (systemd timer). Each run tags unread articles that have no tag yet, choosing
among the existing tags first; the model may create one new tag per article when none fits. The
seed tags and the rules are in [the design doc](docs/superpowers/specs/2026-11-19-wallabag-tagger-design.md);
Readeck specifics in [its own](docs/superpowers/specs/2026-11-21-readeck-support-design.md).

## Readeck

How the tags reach KOReader: the [readeck.koplugin](https://github.com/iceyear/readeck.koplugin)
plugin copies each bookmark's labels into KOReader keywords on download, and KOReader filters
collections by keyword. Set the plugin's "Tags to ignore" to `hors-liseuse` to skip non-reading
material at sync.

| Variable | |
|---|---|
| `READECK_URL` | Base URL of the instance |
| `READECK_TOKEN` | API token (Readeck → Profile → API tokens) with bookmark read and write rights |

Bookmarks whose extraction is still running are left for a later run. Bookmarks without an
article (videos, failed extractions) are tagged from their title, site, URL and description.

## Wallabag

> **Unmaintained:** works, but its author no longer uses Wallabag. Issues and pull requests are
> welcome; nobody tests it day to day.

How the tags reach KOReader: Wallabag exports tags as EPUB subjects, and KOReader filters
collections by keyword. Set the KOReader Wallabag plugin's "Do not download articles with tags" to
`hors-liseuse`.

| Variable | |
|---|---|
| `WALLABAG_URL` | Base URL of the instance |
| `WALLABAG_CLIENT_ID`, `WALLABAG_CLIENT_SECRET` | API client (Wallabag → Developer) |
| `WALLABAG_USERNAME`, `WALLABAG_PASSWORD` | Account |

## Shared configuration

| Variable | |
|---|---|
| `CLAUDE_CODE_OAUTH_TOKEN` | Read by `claude` itself (`claude setup-token`) |
| `STATE_DIRECTORY` | Where `failures.json` lives (set by systemd's `StateDirectory`; default `.`) |
| `CLAUDE_BIN` | `claude` binary (default: `claude` from `PATH`) |

`claude` only receives an allowlist of the environment: never the service credentials.

Exit codes: `0` all good, `1` something was not tagged (see the logs), `2` missing configuration.

## Operating it

- New tags: `journalctl -u readeck-tagger | grep "NEW TAG"` (or `-u wallabag-tagger`).
- An article that fails 3 times is skipped (logged as `skipped` on every run, exit code 1). Delete
  `$STATE_DIRECTORY/failures.json` to retry.
- To have an article re-tagged, remove all its tags (labels).

## Development

```sh
go test ./...
nix build .#readeck-tagger .#wallabag-tagger
```
