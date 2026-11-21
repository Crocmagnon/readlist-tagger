# Readeck support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `readeck-tagger` binary next to the unchanged `wallabag-tagger`, sharing the tagging domain.

**Architecture:** The domain (`internal/domain/tagging`) already talks to the service through the
`Articles` port. We make `EntryID` a string, add an `internal/adapters/readeck` adapter and a
`cmd/readeck-tagger` entry point, and expose both binaries from the flake.

**Tech Stack:** Go 1.27 (stdlib only, no dependencies), Nix flake (`buildGoModule`).

**Spec:** `docs/superpowers/specs/2026-11-21-readeck-support-design.md` (builds on
`docs/superpowers/specs/2026-11-19-wallabag-tagger-design.md`).

## Global Constraints

- Module path `github.com/Crocmagnon/readlist-tagger`.
- No new Go dependency (`vendorHash = null` stays).
- Layout rules in `CLAUDE.md`: the domain never imports an adapter; adapters never import each other; ports take and return domain types or plain values.
- `wallabag-tagger` keeps its binary name, its environment variables and its behaviour.
- `readeck-tagger` variables: `READECK_URL`, `READECK_TOKEN` (required), plus `STATE_DIRECTORY`, `CLAUDE_BIN`, and `CLAUDE_CODE_OAUTH_TOKEN` (read by `claude`). Exit codes 0 / 1 / 2 as in `wallabag-tagger`.
- The tagger only ever adds labels: never remove a label, never change content, title, read or marked state.
- Verification for every task: `go test ./... && go vet ./...`; Task 3 also `nix build .#readeck-tagger .#wallabag-tagger` (new files must be `git add`ed first).

## Review Focus

1. A bookmark with `has_article=false` must not trigger `GET /api/bookmarks/{id}/article` (it 500s in Readeck 0.22.3): test in Task 2.
2. `READECK_TOKEN` must never reach the `claude` subprocess: test in Task 3 (the CLI's env allowlist already excludes it; pin it).
3. More bookmarks than one page: pagination must not drop or repeat any: test in Task 2.
4. A non-2xx answer from Readeck (wrong token → 401) must surface as an error naming the status, not as "no entries": test in Task 2.
5. An existing Wallabag `failures.json` (`{"123": 2}`) must still load with string ids: test in Task 1.

---

### Task 1: Rename the module and make `EntryID` a string

**Files:**
- Modify: `go.mod`, every `.go` import of `github.com/Crocmagnon/wallabag-tagger/...`
- Modify: `internal/domain/tagging/ports.go`, `tags.go`, `run.go`, `tags_test.go`, `run_test.go`
- Modify: `internal/adapters/wallabag/client.go`, `client_test.go`
- Modify: `internal/adapters/failurestore/file_test.go`

**Interfaces:**
- Produces: `type EntryID string` in package `tagging`; `Existing(serviceTags []Tag) []Tag` (parameter renamed, same behaviour).

- [ ] **Step 1: Rename the module path.** `go.mod` → `module github.com/Crocmagnon/readlist-tagger`, and rewrite every import. Run `go build ./...`: expected success.
- [ ] **Step 2: Write the failing tests.**
  - `failurestore`: `TestLoadWallabagNumericKeys`: write `{"123": 2}` to a temp file, `Load` it, assert `Count(tagging.EntryID("123")) == 2`.
  - `wallabag`: in the existing `UnreadEntries` test, assert the returned entry's `ID == tagging.EntryID("42")` for an item with `"id": 42`; in the `AddTags` test, call it with `EntryID("42")` and assert the request path is `/api/entries/42/tags.json`.
  - Domain tests: replace integer ids with strings (`EntryID("a")`, …).
- [ ] **Step 3: Run `go test ./...`**: expected compile failure (`EntryID` is still `int`).
- [ ] **Step 4: Implement.** `type EntryID string`. Wallabag adapter: `strconv.Itoa(it.ID)` into `EntryID`; `AddTags` builds the path with `%s` (the id is digits, no escaping needed). Generic wording: package doc ("the business rules of readlist-tagger"), `Tag` ("a tag label as stored by the service"), `Entry.Content` ("article HTML as stored by the service"), `Existing`'s parameter and `run.go`'s local variable `wallabagTags` → `serviceTags`, test name `TestExistingMergesSeedsAndServiceTags`.
- [ ] **Step 5: Run `go test ./... && go vet ./...`**: expected PASS.
- [ ] **Step 6: Commit.** `refactor: module readlist-tagger, string entry ids`

### Task 2: Readeck adapter

**Files:**
- Create: `internal/adapters/readeck/client.go`
- Test: `internal/adapters/readeck/client_test.go`

**Interfaces:**
- Consumes: `tagging.Articles`, `tagging.Entry`, `tagging.EntryID`, `tagging.Tag`, `tagging.Decision` (Task 1).
- Produces: `func New(baseURL, token string, hc *http.Client) *Client`, with `var _ tagging.Articles = (*Client)(nil)`.

Readeck API (0.22.3, all under `/api`, header `Authorization: Bearer <token>`, JSON):
- `GET /api/bookmarks/labels` → `[{"name": "go", ...}]`
- `GET /api/bookmarks?is_archived=false&has_labels=false&is_loaded=true&limit=N&offset=M` → `[{"id": "...", "title": "...", "url": "...", "site": "...", "description": "...", "has_article": true, "labels": []}]`; header `Total-Count`.
- `GET /api/bookmarks/{id}/article` → `text/html` body.
- `PATCH /api/bookmarks/{id}` with `{"add_labels": ["go", "ia"]}`.

- [ ] **Step 1: Write the failing tests** against an `httptest.Server` that checks the bearer (`Bearer tok`) on every request, on the model of `internal/adapters/wallabag/client_test.go`:
  - `TestTagsListsLabelNames`: labels `[{"name":"go"},{"name":"ia"}]` → `[]Tag{"go","ia"}`.
  - `TestUnreadEntriesSendsFilters`: assert query `is_archived=false`, `has_labels=false`, `is_loaded=true`.
  - `TestUnreadEntriesPaginates`: 3 bookmarks served 2 per page (honour `limit`/`offset`, set `Total-Count: 3`), adapter built with a page size of 2 (see Step 3) → 3 entries, ids in order, no duplicate.
  - `TestUnreadEntriesFetchesArticleOnlyWhenPresent`: bookmark `a` with `has_article: true` (article route returns `<p>body</p>`) and bookmark `b` with `has_article: false, description: "a video"`; the handler fails the test if `/api/bookmarks/b/article` is requested. Assert `a.Content == "<p>body</p>"`, `b.Content == "a video"`, `Domain` taken from `site`, `URL` and `Title` mapped.
  - `TestUnreadEntriesArticleErrorFails`: article route returns 500 for a `has_article: true` bookmark → `UnreadEntries` returns an error.
  - `TestNon2xxIsError`: 401 on `/api/bookmarks/labels` → error containing `401`.
  - `TestAddTagsPatchesAddLabels`: `AddTags(ctx, "a", d)` with a decision for `go` and `ia` (build it via `tagging.Answer{Tags: []string{"go","ia"}}.Validate([]tagging.Tag{"go","ia"})`) → `PATCH /api/bookmarks/a`, `Content-Type: application/json`, body `{"add_labels":["go","ia"]}`.
- [ ] **Step 2: Run `go test ./internal/adapters/readeck/`**: expected FAIL (package does not exist).
- [ ] **Step 3: Implement `Client`** in `client.go`: `New(baseURL, token string, hc *http.Client) *Client` (trim the trailing `/`), methods `Tags`, `UnreadEntries`, `AddTags`, a private `do` helper like the Wallabag one but sending/receiving JSON and returning `status N: <first 200 bytes>` on non-2xx. Page size is an unexported field defaulting to 100, settable from the test file (same package). Stop paginating when a page returns fewer than the page size. Bookmark ids go through `url.PathEscape` when building paths.
- [ ] **Step 4: Run `go test ./... && go vet ./...`**: expected PASS.
- [ ] **Step 5: Commit.** `feat: Readeck adapter`

### Task 3: `readeck-tagger` entry point, flake and docs

**Files:**
- Create: `cmd/readeck-tagger/main.go`
- Modify: `flake.nix`, `README.md`, `CLAUDE.md`
- Modify: `internal/adapters/claude/cli.go` (comment only), `internal/adapters/claude/cli_test.go`

**Interfaces:**
- Consumes: `readeck.New` (Task 2), `failurestore.Load`, `claude.CLI`, `tagging.Run`.

- [ ] **Step 1: Write the failing test.** In `TestClassifyPassesOnlyAllowedEnv`, also `t.Setenv("READECK_TOKEN", "secret-token")`; the existing `secret-` assertion covers it. Run `go test ./internal/adapters/claude/`: expected PASS already (allowlist); keep the test as the pin. Update the `envAllowlist` comment to "never the service credentials".
- [ ] **Step 2: Write `cmd/readeck-tagger/main.go`**: a copy of `cmd/wallabag-tagger/main.go` with required variables `READECK_URL`, `READECK_TOKEN`, `readeck.New(url, token, &http.Client{Timeout: 60 * time.Second})`, no login step, package doc "Command readeck-tagger tags unlabeled unread Readeck bookmarks by theme with Claude." Same logger, same `claude.CLI{… Model: "opus", Effort: "low", Timeout: 3 * time.Minute}`, same exit codes.
- [ ] **Step 3: Check it.** `go build ./... && go vet ./...`, then `env -u READECK_URL go run ./cmd/readeck-tagger; echo $?`: expected `missing environment variables: READECK_URL, READECK_TOKEN` and `2`.
- [ ] **Step 4: Flake.** Description "Auto-tag unread read-it-later articles (Readeck, Wallabag) by theme with Claude". Factor the `buildGoModule` call into a local function taking the binary name, producing `wallabag-tagger` (`subPackages = [ "cmd/wallabag-tagger" ]`) and `readeck-tagger` (`subPackages = [ "cmd/readeck-tagger" ]`); `default = readeck-tagger`. `git add` the new files, then `nix build .#readeck-tagger .#wallabag-tagger`: expected success, `result/bin/readeck-tagger` and `result-1/bin/wallabag-tagger`.
- [ ] **Step 5: Docs.**
  - `README.md`: title `readlist-tagger`; intro on read-it-later services; how tags reach KOReader per backend (Readeck: the `readeck.koplugin` plugin copies labels into KOReader keywords, "Tags to ignore" → `hors-liseuse`; Wallabag: EPUB subjects, plugin's "Do not download articles with tags"); a configuration table per binary (Readeck: `READECK_URL`, `READECK_TOKEN` = an API token from Readeck → Profile → API tokens with bookmark read/write); shared variables; the Wallabag section opens with: "Unmaintained: works, but its author no longer uses Wallabag." Operating notes: `journalctl -u readeck-tagger`, re-tag by removing every label.
  - `CLAUDE.md`: layout lists both `cmd/` entry points; Design section lists both specs; deployment points at `hosts/srv-1/services/readeck-tagger.nix`.
- [ ] **Step 6: Run `go test ./... && go vet ./...`**: expected PASS.
- [ ] **Step 7: Commit.** `feat: readeck-tagger entry point, flake packages and docs`

## After merge (not in this repo)

nixos-config PR, per the spec's Deployment section: flake input `readlist-tagger`, overlay exposing
`readeck-tagger`, `readeck-tagger.nix` replacing `wallabag-tagger.nix`, secret
`readeck-tagger-env.age` (Gabriel creates it).
