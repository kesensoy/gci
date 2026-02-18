## CLAUDE.md — GCI (Git Checkout Issue)

This file gives Claude Code everything it needs to contribute productively to this repo, with a clear model of the current system, safe command allowlists, and a prioritized backlog. It is tuned for teammates who may not be familiar with Go.

References used:
- Anthropic: "Claude Code: Best practices for agentic coding" — https://www.anthropic.com/engineering/claude-code-best-practices
- Dzombak: "Getting good results from Claude Code" — https://www.dzombak.com/blog/2025/08/getting-good-results-from-claude-code/


### Objectives and invariants

- Keep a green build (`go build`) and tests (`go test ./...`) after each logical edit.
- Require explicit configuration via `gci setup` or env vars; no company-specific defaults.
- Prefer small PR-sized edits with explicit verification steps.
- Non-Go contributors should be able to: download the binary, run the app, and—when working with Claude—follow guided prompts.


### Allowed tools (recommendations)

Default to conservative permissions. Explicitly allow these for faster iteration:
- Edit: file edits within this repo
- Bash(go build): `go build -o gci .`
- Bash(go test): `go test ./...`
- Bash(git commit:*): commit creation (no pushes by default)
- Bash(git checkout|git switch|git rev-parse): safe git ops
- Bash(run app): `./gci`, `./gci board`, `./install-user.sh`
- Bash(op read:*): read-only 1Password secret used by this app
- Optional (scoped): `gh pr create/view` or `glab mr create/view`

Ask each time or deny: destructive filesystem operations, package manager changes, or commands outside this repo unless necessary.


### Environment and quickstart

- Go ≥ 1.19, Git, 1Password CLI (`op signin`) or `JIRA_API_TOKEN` env var, Claude CLI (optional, for Interactive Mode and `gci create`), JIRA access to your configured instance.
- Build: `go build -o gci .`
- Run: `./gci`
- TUI board: `./gci board`
- First-time setup: `./gci setup` or press `w` in board view
- User install: `./install-user.sh`
- Help: `./gci --help`
- First-time dev setup: `make hooks` (installs pre-push reminder)

Use the slash command `/project:dev-quickstart` to validate your local setup.


### Release workflow

Patch releases are automatic. When Go source files (outside `vendor/`) are pushed to `main`:
1. `.github/workflows/auto-release.yml` detects the change vs the latest `v*` tag
2. It auto-increments the patch version, pushes a new annotated tag (e.g., `v1.0.0` → `v1.0.1`), and runs GoReleaser in the same job
3. GoReleaser builds binaries and publishes a GitHub Release
4. Users running `gci update` will see the new version

The workflow also supports `workflow_dispatch`, so it can be triggered manually from the GitHub Actions UI. The same Go-change guard applies — no release is created unless there are actual source changes since the last tag.

`.github/workflows/release.yml` handles manually-pushed tags only (for major/minor bumps).

**Docs-only changes do not trigger releases.** The workflow only acts if `.go` files changed since the latest tag.

For minor or major version bumps, create the tag manually before pushing:
```bash
make tag VERSION=1.1.0          # creates v1.1.0 annotated tag
git push origin v1.1.0
```

A local pre-push hook (`make hooks`) prints a reminder when pushing Go changes to main.

**Dev builds vs release builds:** `go build -o gci .` produces a dev binary (`Version=dev`) that refuses self-update — this is intentional. Release binaries have the version baked in via ldflags by GoReleaser (`.goreleaser.yml`) or `install-user.sh`. To test the end-user update path locally, use `./install-user.sh` which injects the latest git tag as the version.


### Current system (post-implementation)

Core features:
- TOML config at `~/.config/gci/config.toml` with validation
- `ErrNotConfigured` sentinel prompts `gci setup` when no config exists
- JIRA board discovery helper with caching and ranking
- Setup wizard available via `gci setup` and TUI hotkey `w`
- **Branch mode** (`b` key): creates/checkouts branch for selected issue
- **Interactive Mode** (`Enter` key): configurable workflow (worktrees + Claude optional)
- **Reverse workflow** (`gci create`): generate JIRA ticket from current changes using Claude, auto-rename branch
- **Optional Claude integration**: `enable_claude` config; auto-detected during setup
- **Optional worktrees**: `enable_worktrees` config; controls Interactive Mode behavior
- Hardcoded dark theme, vim-style keys, always-on fuzzy search

Repo map:
- `internal/usercfg/` - config, defaults, fuzzy search
- `internal/jira/` - discovery, board API
- `main.go` - CLI commands, worktree functions, Claude spawn, branch naming, `gci create`
- `board_tui.go` - Kanban TUI, hardcoded styles/keys, Interactive Mode (Enter key)


### Configuration schema

`~/.config/gci/config.toml`:
```toml
projects = ["PROJ1", "PROJ2"]
default_scope = "assigned_or_reported"  # assigned_or_reported|assigned|reported|unassigned
jira_url = "https://your-company.atlassian.net"
enable_claude = false     # auto-detected during gci setup; enables Claude AI integration
enable_worktrees = true   # enables git worktrees for Interactive Mode (Enter key)

[boards]
PROJ1_kanban = 123
PROJ2_scrum = 456

# Optional: 1Password path for JIRA API token
# op_jira_token_path = "op://VaultName/ItemName/credential"

# Optional: email domain aliases
# [email_domain_map]
# "old-domain.com" = "new-domain.com"
```

**Removed fields:**
- `theme` (dark theme hardcoded)
- `key_mappings` (vim-style hardcoded)
- `jql_presets` (feature removed)
- `branch_name_template` (kebab-case hardcoded)

Notes:
- Discovery results are cached at `~/.config/gci_boards_cache.json`.

Loading order and fallbacks:
- Runtime config (TOML) → env var overlays → `ErrNotConfigured` if no config exists.
- If load fails, warn and use empty defaults; setup guard in `loadConfig()` prompts user.


### Prompts and working style

- Small, iterative edits; keep a green build after each step.
- Provide concrete acceptance criteria (e.g., “build OK, tests OK, TUI launches and navigates”).
- Specify exact files/functions to modify; avoid vague goals.
- Use `--help` for unknown tools and propose commands before long jobs.
- Use `/clear` between distinct tasks to reset context (per Anthropic best practices).
- For large tasks, maintain a Markdown checklist; apply changes one item at a time.
- For verification, run a separate Claude session to review changes independently.


### Conventions (Go)

- Descriptive identifiers; functions are verbs; variables are nouns.
- Guard clauses; avoid deep nesting.
- Return errors; avoid panics for control flow.
- Do not reformat unrelated code.


### Comprehensive product and engineering backlog (status)

- Epic A: Configuration & Setup — completed
- Epic B: TUI UX & Accessibility — completed (help overlay scrollable, themes, key remap, UI prefs, fuzzy search)
- **Epic C: Bloat Removal — completed** (theme, keymapping, JQL, templates removed; dark + vim hardcoded)
- **Epic D: Worktree Integration — completed** (worktree workflow via Interactive Mode, `Enter` key)
- **Epic E: Claude Integration — completed** (auto-spawn with ticket context via Interactive Mode; reverse workflow via `gci create`)
- **Epic F: Release Automation — completed** (auto-tag on Go changes, pre-push hook, CLAUDE.md docs)
- Epics G–I: deferred unless prioritized


### Automation and logging guidance (from best practices)

- Use checklists and a scratchpad when tackling multi-step or broad changes. Keep a `backlog.md` or `checklist.md` in-repo for large efforts. ([Anthropic best practices](https://www.anthropic.com/engineering/claude-code-best-practices))
- Reset context frequently with `/clear` between tasks.
- For automated/backlog execution, prefer headless mode where appropriate:
  - Example (single task):
    - `claude -p "Execute backlog item X; when done return OK or FAIL." --allowedTools Edit Bash(go build:*) Bash(go test:*) Bash(git commit:*) --output-format stream-json`
  - Example (script loop): generate a list of tasks, iterate and call headless Claude with explicit success criteria, collect `OK/FAIL` results.
- Always propose commands before running long jobs; prefer non-interactive flags and piping to avoid stuck sessions.
- Logging in code: adopt a `--verbose` flag with structured logging. Default remains quiet; never log secrets. Direct user-facing errors should include remediation.


### Security and secrets

- 1Password access is read-only via configured `op_jira_token_path`.
- `JIRA_API_TOKEN` env var provides a non-1Password auth path.
- Never log secrets. Avoid printing the email/token.
- For network calls, set conservative timeouts and handle errors gracefully.


### Headless ideas

- Lint-like subjective review: scan names/comments for clarity; produce a checklist and apply iteratively.
- Draft PR descriptions by summarizing diffs and checklists.

