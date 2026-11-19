# wallabag-tagger

Tags every untagged unread article of a [Wallabag](https://wallabag.org) instance by theme, with
Claude (`claude -p`), so the articles can be filtered by theme offline on a KoReader e-reader:
Wallabag exports tags as EPUB subjects, and KoReader filters collections by keyword.

Run it every minute (systemd timer). Each run tags unread articles that have no tag yet, choosing
among the existing tags first; the model may create one new tag per article when none fits. The
seed tags and the rules are in [the design doc](docs/superpowers/specs/2026-11-19-wallabag-tagger-design.md).

## Configuration

Environment variables:

| Variable | |
|---|---|
| `WALLABAG_URL` | Base URL of the instance |
| `WALLABAG_CLIENT_ID`, `WALLABAG_CLIENT_SECRET` | API client (Wallabag → Developer) |
| `WALLABAG_USERNAME`, `WALLABAG_PASSWORD` | Account |
| `CLAUDE_CODE_OAUTH_TOKEN` | Read by `claude` itself (`claude setup-token`) |
| `STATE_DIRECTORY` | Where `failures.json` lives (set by systemd's `StateDirectory`; default `.`) |
| `CLAUDE_BIN` | `claude` binary (default: `claude` from `PATH`) |

Exit codes: `0` all good, `1` something was not tagged (see the logs), `2` missing configuration.

## Operating it

- New tags: `journalctl -u wallabag-tagger | grep "NEW TAG"`.
- An article that fails 3 times is skipped (logged as `skipped` on every run, exit code 1). Delete
  `$STATE_DIRECTORY/failures.json` to retry.
- To have an article re-tagged, remove all its tags.

## Development

```sh
go test ./...
nix build
```
