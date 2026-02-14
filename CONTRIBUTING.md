# Contributing to GCI

Thank you for your interest in contributing to GCI! This guide will help you get started.

## Development Setup

### Prerequisites
- **Go 1.19+** - [Install Go](https://golang.org/doc/install)
- **Git** - For version control
- **Make** - For build automation (optional)

### Getting Started
```bash
# Clone the repository
git clone <repository-url>
cd gci

# Build the project
make build
# or
go build -o gci .

# Run tests
make test
# or
go test ./...

# Install for development
make install
```

## Project Structure

```
gci/
├── main.go                    # CLI entry point and core logic
├── board_tui.go              # TUI implementation (Bubble Tea)
├── internal/
│   ├── errors/               # User-friendly error handling
│   ├── httputil/             # HTTP client with retry logic
│   ├── jira/                 # JIRA API integration
│   ├── logger/               # Structured logging
│   ├── usercfg/             # Configuration management
│   └── version/             # Version information
├── examples/                 # Configuration examples
├── scripts/                  # Build scripts
└── Makefile                 # Build automation
```

## Coding Standards

### Go Style Guide
- Follow standard Go conventions
- Use `gofmt` for formatting
- Use `golangci-lint` for linting (config in `.golangci.yml`)
- Write descriptive function and variable names
- Functions should be verbs, variables should be nouns

### Code Organization
- Keep functions focused and small
- Use guard clauses to avoid deep nesting
- Return errors rather than using panics for control flow
- Prefer explicit error handling over silent failures

### Example Code Style
```go
// Good: descriptive function name, guard clause, explicit error handling
func fetchUserIssues(userEmail string) ([]JiraIssue, error) {
    if userEmail == "" {
        return nil, errors.NewInvalidInputError("email cannot be empty")
    }
    
    issues, err := client.GetIssues(userEmail)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch issues: %w", err)
    }
    
    return issues, nil
}
```

### Error Handling
- Use the `internal/errors` package for user-facing errors
- Include remediation hints in error messages
- Wrap errors with context using `fmt.Errorf`
- Never expose internal errors directly to users

### Logging
- Use the `internal/logger` package for all logging
- Log at appropriate levels (DEBUG, INFO, WARN, ERROR)
- Never log sensitive information (tokens, passwords)
- Use structured logging with context

## Testing

### Running Tests
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detection
go test -race ./...

# Run specific test
go test ./internal/usercfg -v
```

### Writing Tests
- Write unit tests for all new functionality
- Use table-driven tests for multiple scenarios
- Test error conditions and edge cases
- Mock external dependencies when appropriate

### Test Structure
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid input",
            input:    "test",
            expected: "result",
            wantErr:  false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := FunctionName(tt.input)
            
            if tt.wantErr && err == nil {
                t.Error("expected error but got none")
            }
            
            if !tt.wantErr && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            
            if result != tt.expected {
                t.Errorf("expected %q, got %q", tt.expected, result)
            }
        })
    }
}
```

## Pull Request Guidelines

### PR Size and Scope
- Keep PRs focused on a single feature or fix
- Aim for PRs with fewer than 500 lines of changes
- Break large features into smaller, reviewable chunks
- Include tests for new functionality

### PR Description Template
```markdown
## Summary
Brief description of what this PR does.

## Changes
- List of specific changes made
- Any breaking changes
- New features added

## Testing
- [ ] Unit tests added/updated
- [ ] Manual testing completed
- [ ] All existing tests pass

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated (if needed)
- [ ] Commit messages follow conventions
```

### Before Submitting
1. **Build and test locally:**
   ```bash
   make build
   make test
   ./gci version  # Verify it works
   ```

2. **Run linting:**
   ```bash
   golangci-lint run
   ```

3. **Check for unused code:**
   ```bash
   go mod tidy
   ```

## Commit Message Conventions

### Format
```
type(scope): brief description

Longer explanation if needed.

- Additional details
- Reference issues: Fixes #123
```

### Types
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

### Examples
```
feat(tui): add board refresh on scope change

Automatically refresh board data when switching scopes
via the 's' key.

- Triggers background fetch on scope change
- Shows loading indicator during refresh
- Preserves cursor position

Fixes #456
```

## Development Workflow

### Feature Development
1. Create a feature branch: `git checkout -b feature/your-feature`
2. Make changes in small, logical commits
3. Add tests for new functionality
4. Update documentation if needed
5. Run tests and ensure they pass
6. Create a pull request

### Bug Fixes
1. Create a bug fix branch: `git checkout -b fix/issue-description`
2. Write a failing test that reproduces the bug
3. Fix the bug
4. Ensure the test now passes
5. Create a pull request with "Fixes #issue-number"

## Architecture Guidelines

### Adding New Features
- Consider the user experience first
- Follow existing patterns and conventions
- Add configuration options when appropriate
- Include help text and documentation
- Consider performance implications

### TUI Development
- Use the existing Bubble Tea patterns
- Update help overlays for new keybindings
- Ensure responsive design for different terminal sizes
- Test with different terminal sizes

### Configuration Changes
- Update the schema version if needed
- Provide migration logic in `internal/usercfg`
- Update example configuration files
- Document new options in README

## Getting Help

- Check existing issues and discussions
- Review the codebase for similar patterns
- Ask questions in pull request reviews
- Follow the project's coding standards

## License

By contributing to GCI, you agree that your contributions will be licensed under the same license as the project.