# CLAUDE.md

## Commands

```sh
go test ./...
go vet ./...
nix build   # only sees git-tracked files: `git add` new files first
```

## Layout

- `cmd/wallabag-tagger/`: entry point. Reads the environment, builds the adapters, hands them to
  the domain.
- `internal/domain/`: business code (sub-packages if needed). It never imports an adapter: it
  declares the ports it needs (interfaces) and receives the adapters.
- `internal/adapters/`: everything that talks to the outside world (API clients, CLIs, files,
  databases). Adapters never import each other.
- Functions the domain calls (its ports, implemented by adapters) only take and return domain
  types or plain values. Constructors and setup called only by `cmd/` may take adapter-specific
  types (e.g. `wallabag.Credentials`).
- Domain types are strongly typed so that invalid states do not compile where possible (e.g.
  `EntryID`, `Tag`, and `Decision`, which only `Answer.Validate` can build and is never empty).

## Design

`docs/superpowers/specs/2026-11-19-wallabag-tagger-design.md`. Deployed on srv-1 by
`Crocmagnon/nixos-config` (`hosts/srv-1/services/wallabag-tagger.nix`).
