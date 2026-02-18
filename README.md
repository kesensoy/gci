# GCI - Git Checkout Issue

A fast, single-binary CLI tool for working with JIRA issues from the terminal — browse, branch, and create tickets with an interactive Kanban board. Built in Go.

## Features

- **Single binary** — no runtime dependencies
- **Secure auth** — 1Password, env var, or any token source
- **Multi-project** — query across JIRA projects
- **Interactive TUI** — Kanban board with fuzzy search, vim keys
- **Reverse workflow** — `gci create` generates a JIRA ticket from your current changes
- **Worktree + Claude integration** — optional, configurable via `gci setup`
- **Smart branch naming** — `ISSUE-123_summary-in-kebab-case`
- **Self-update** — `gci update`

## Installation

### From GitHub Releases

Download the latest binary from [Releases](../../releases), then:

```bash
chmod +x gci
mv gci ~/.local/bin/   # or anywhere in your PATH
gci setup              # first-time configuration
```

### Build from Source

Requires Go 1.19+:

```bash
go build -o gci .
mv gci ~/.local/bin/
gci setup
```

### Self-Update

```bash
gci update
```

## Usage

### Browse Issues

```bash
gci                # list issues across all configured projects
gci -a             # include unassigned issues
gci -p MYPROJECT   # filter to one project
```

### Kanban Board

```bash
gci board
```

### Manage Configuration

```bash
gci config doctor    # check config health and connectivity
gci config print     # display current config
gci config path      # show config file location
gci config get KEY   # get a specific config value
gci config set KEY VALUE  # set a config value
gci config migrate   # migrate config to latest schema
```

### Create a Ticket (Reverse Workflow)

Already started work and need a ticket? `gci create` analyzes your branch's changes, uses Claude to suggest a title and description, creates the JIRA issue, and renames your branch to match.

```bash
gci create                # full interactive flow
gci create --dry-run      # preview without creating anything
gci create -P MYPROJECT   # target a specific project
```

### Board Key Bindings

| Key | Action |
|-----|--------|
| `hjkl` / arrows | Navigate |
| `tab` / `shift+tab` | Switch column |
| `/` | Filter (fuzzy search) |
| `enter` | Interactive mode (branch, worktree, Claude — based on config) |
| `b` | Create/checkout branch for selected issue |
| `s` | Cycle scope |
| `r` | Refresh |
| `o` | Open in browser |
| `w` | Setup wizard |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |

### Interactive Mode (`Enter` key)

Pressing `Enter` on a board issue runs the configurable workflow:

| Config | Behavior |
|--------|----------|
| Default | Creates/checks out a branch |
| `enable_worktrees = true` | Creates an isolated git worktree in a sibling directory |
| `enable_claude = true` | Spawns Claude CLI with full ticket context |

Both options are auto-detected during `gci setup`. Branch naming follows `ISSUE-123_summary-in-kebab-case`.

## Prerequisites

- **Git** (configured with your email)
- **JIRA account** with API token access
- **1Password CLI** *(optional)* — for token retrieval; env var works too
- **Claude CLI** *(optional)* — for `gci create` and Interactive Mode's Claude integration
- **Go 1.19+** *(build from source only)*

## Configuration

### Quick Setup

```bash
gci setup
```

Or press `w` in the board view. The wizard walks through projects, JIRA URL, board discovery, and optional integrations (worktrees, Claude).

### Configuration File

`~/.config/gci/config.toml`:

```toml
schema_version = 1
projects = ["MYPROJECT", "INFRA"]
default_scope = "assigned_or_reported"
jira_url = "https://your-company.atlassian.net"
enable_claude = false
enable_worktrees = true

[boards]
MYPROJECT_kanban = 123
INFRA_scrum = 456
```

See [`examples/gci.toml`](examples/gci.toml) for a complete annotated example.

### Authentication

1. **Create a JIRA API token** at [Atlassian API Tokens](https://id.atlassian.com/manage-profile/security/api-tokens)
2. **Provide the token** (choose one):
   - **Environment variable:** `export JIRA_API_TOKEN=your-token`
   - **1Password:** store it and configure the path during `gci setup`
3. **Verify:** `gci config doctor`

GCI reads your email from `git config user.email`. If your git email domain differs from JIRA, configure a mapping:

```toml
[email_domain_map]
"old-domain.com" = "jira-domain.com"
```

## Troubleshooting

### "Failed to get git user email"
```bash
git config --global user.email "your.email@example.com"
```

### "No JIRA API token found"
Provide a token via one of:
1. `export JIRA_API_TOKEN=your-token`
2. Configure `op_jira_token_path` in your config and run `op signin`

### "Command not found: gci"
```bash
export PATH="$HOME/.local/bin:$PATH"
# Add to ~/.zshrc or ~/.bashrc to make permanent
```
