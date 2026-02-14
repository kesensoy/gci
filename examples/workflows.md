# GCI Workflow Examples

## Quick Start

```bash
gci setup             # Interactive configuration wizard
gci config doctor     # Verify configuration health
gci board             # Open personal Kanban board
```

## Basic Workflows

### Quick Issue Branch Creation
```bash
gci                      # Select from your assigned/reported issues
git push -u origin HEAD  # Push the new branch
```

### Using the Kanban Board
```bash
gci board               # Open interactive board
# Navigate with hjkl or arrows
# Press 'b' to create worktree + spawn Claude
# Press 'o' to open issue in browser
# Press '/' to filter issues (fuzzy search)
```

## Configuration

### Change Settings
```bash
gci config set default_scope assigned   # Change default scope
gci config get projects                 # View current projects
gci config print                        # Show full effective config
```

### Environment Overrides
```bash
export GCI_PROJECTS="PROJ1,PROJ2"
export GCI_DEFAULT_SCOPE="assigned"
gci board  # Uses environment overrides
```

## Troubleshooting

### Diagnostic Commands
```bash
gci config doctor     # Check configuration health
gci --verbose board   # Enable debug logging
gci version           # Show version info
```

### Performance Tips
- Board discovery cached for 24 hours
- Use `/` for local filtering instead of new API queries
