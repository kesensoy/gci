package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"gci/internal/errors"
	"gci/internal/httputil"
	"gci/internal/jira"
	"gci/internal/logger"
	"gci/internal/usercfg"
	"gci/internal/version"

	"github.com/AlecAivazis/survey/v2"
	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

type JiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description *struct {
			Content []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				} `json:"content,omitempty"`
			} `json:"content,omitempty"`
		} `json:"description"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		IssueType struct {
			Name    string `json:"name"`
			Subtask bool   `json:"subtask"`
		} `json:"issuetype"`
		Parent struct {
			Key string `json:"key"`
		} `json:"parent"`
		Status struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Name string `json:"name"`
			} `json:"statusCategory"`
		} `json:"status"`
		Assignee struct {
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		} `json:"assignee"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
	} `json:"fields"`
}

type JiraResponse struct {
	Issues []JiraIssue `json:"issues"`
	Total  int         `json:"total"`
}

type WorktreeResult struct {
	Path       string
	BranchName string
	Created    bool
	Error      error
}

type Config struct {
	JiraURL         string
	Email           string
	APIToken        string
	Projects        []string
	All             bool
	EnableClaude    bool
	EnableWorktrees bool
}

var updateCheckCh <-chan version.UpdateCheckResult

var rootCmd = &cobra.Command{
	Use:   "gci",
	Short: "Create Git branch from JIRA issue",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logger.SetVerbose(verbose)

		name := cmd.Name()
		if name != "update" && name != "version" {
			updateCheckCh = version.StartUpdateCheck()
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if updateCheckCh == nil {
			return
		}
		select {
		case result := <-updateCheckCh:
			if result.NewVersion != "" {
				fmt.Fprintf(os.Stderr, "\n\033[33mA new version of gci is available: %s (current: %s)\033[0m\n", result.NewVersion, version.GetShortVersion())
				fmt.Fprintf(os.Stderr, "\033[33mRun 'gci update' to upgrade.\033[0m\n")
			}
		case <-time.After(500 * time.Millisecond):
		}
	},
	Run: runGCI,
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure GCI settings interactively",
	Long:  "Launch a setup wizard to configure projects, boards, and default scope for GCI",
	Run:   runSetup,
}

// configCmd provides config management subcommands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage GCI configuration",
	Long:  "Commands for managing GCI configuration files, migrations, and settings",
}

var configMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate config file to current schema version",
	Long:  "Load the config file, apply any necessary schema migrations, and save it back to disk with the current schema version",
	Run:   runConfigMigrate,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the path to the configuration file",
	Long:  "Display the path where GCI looks for its configuration file (XDG-compliant location)",
	Run:   runConfigPath,
}

var configPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print the current configuration",
	Long:  "Display the current effective configuration, including defaults and environment variable overlays",
	Run:   runConfigPrint,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long:  "Retrieve and display a specific configuration value. Keys: projects, default_scope, jira_url, boards",
	Args:  cobra.ExactArgs(1),
	Run:   runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long:  "Set a configuration value and save to file. Keys: default_scope, jira_url. Use 'gci setup' for projects and boards.",
	Args:  cobra.ExactArgs(2),
	Run:   runConfigSet,
}

var configDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration health",
	Long:  "Validate configuration file, check for common issues, and suggest fixes",
	Run:   runConfigDoctor,
}

// versionCmd displays version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  "Display version, build information, and platform details for GCI",
	Run:   runVersion,
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Self-update gci to the latest release",
	Long:  "Check GitHub Releases for a newer version of gci and replace the current binary.",
	Run:   runUpdate,
}

// boardCmd launches a TUI showing a personal Kanban view of JIRA issues
var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Open a personal Kanban (To Do / In Progress / Done) for your JIRA issues",
	Long: `Open a personal Kanban board scoped to you across your configured projects.

Controls:
  - Arrows / h j k l: Move selection
  - Tab / Shift+Tab: Switch column
  - r: Refresh
  - s: Cycle scope (Assigned to Me / Reported by Me / Unassigned)
  - /: Filter
  - o: Open selected issue in browser
  - b: Create/checkout a git branch for selected issue
  - w: Open setup wizard
  - q: Quit`,
	Example: "gci board",
	Run:     runBoard,
}

var (
	allFlag     bool
	projectFlag string
	verbose     bool
)

// create command flags
var (
	createProjectFlag string
	createIssueType   string
	createNoRename    bool
	createDryRun      bool
	createModel       string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a JIRA ticket from your current changes",
	Long: `Analyze your current git changes, generate a ticket suggestion using Claude,
create a JIRA issue, and rename your branch to match.

Useful when you've done work first and need a ticket after the fact.`,
	Example: `  gci create                # full interactive flow
  gci create --dry-run      # preview without creating ticket
  gci create -P INF         # target a specific project
  gci create --no-rename    # create ticket but keep current branch name`,
	Run: runCreate,
}

func init() {
	rootCmd.Flags().BoolVarP(&allFlag, "all", "a", false, "Query all open or in-progress issues, not just those reported by the user")

	// Build the help text dynamically based on available projects (including env vars)
	availableProjects := usercfg.GetAvailableProjectsFromRuntime()
	projectChoices := strings.Join(availableProjects, ", ")
	projectHelp := fmt.Sprintf("Which project to query: %s (default: both)", projectChoices)
	rootCmd.Flags().StringVarP(&projectFlag, "project", "p", "both", projectHelp)
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	// Add subcommands
	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(createCmd)

	// create command flags
	createCmd.Flags().StringVarP(&createProjectFlag, "project", "P", "", "Target JIRA project (e.g. INF, CHANGE)")
	createCmd.Flags().StringVarP(&createIssueType, "type", "t", "Task", "JIRA issue type (default: Task)")
	createCmd.Flags().BoolVar(&createNoRename, "no-rename", false, "Create ticket without renaming the current branch")
	createCmd.Flags().BoolVar(&createDryRun, "dry-run", false, "Preview what would be created without making changes")
	createCmd.Flags().StringVarP(&createModel, "model", "m", "haiku", "Claude model for suggestion (e.g. haiku, sonnet, opus)")

	// Add config subcommands
	configCmd.AddCommand(configMigrateCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configPrintCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configDoctorCmd)

	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\n\033[93mOperation cancelled by user.\033[0m")
		os.Exit(0)
	}()
}

// Legacy function removed - now using internal/logger package

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runGCI(cmd *cobra.Command, args []string) {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	issues, err := fetchIssues(config)
	if err != nil {
		log.Fatalf("Failed to fetch issues: %v", err)
	}

	if len(issues) == 0 {
		fmt.Println("\033[93mNo issues found matching the criteria.\033[0m")
		return
	}

	fmt.Printf("Found %d Open, Change Approved, or In Progress issue(s). (Max 10)\n", len(issues))

	selectedIssue, err := selectIssue(issues)
	if err != nil {
		fmt.Println("\n\033[93mOperation cancelled by user.\033[0m")
		return
	}

	branchName := createBranchName(selectedIssue)

	if err := createOrCheckoutBranch(branchName); err != nil {
		log.Fatalf("Failed to create/checkout branch: %v", err)
	}
}

func loadConfig() (*Config, error) {
	// Load user configuration
	userConfig := usercfg.GetRuntimeConfig()

	// Guard: require configuration
	if userConfig.JiraURL == "" || len(userConfig.Projects) == 0 {
		fmt.Println("GCI is not configured yet.")
		fmt.Println()
		fmt.Println("Run:  gci setup")
		fmt.Println()
		fmt.Println("Or set environment variables:")
		fmt.Println("  GCI_JIRA_URL=https://your-company.atlassian.net")
		fmt.Println("  GCI_PROJECTS=PROJ1,PROJ2")
		os.Exit(1)
	}

	// Get email from git config
	emailCmd := exec.Command("git", "config", "user.email")
	emailOutput, err := emailCmd.Output()
	if err != nil {
		return nil, errors.NewGitConfigError(err)
	}
	email := strings.TrimSpace(string(emailOutput))
	// Apply email domain aliases from config
	for oldDomain, newDomain := range userConfig.EmailDomainMap {
		email = strings.Replace(email, oldDomain, newDomain, 1)
	}

	// Get API token: env var > 1Password (configured path)
	var apiToken string
	readSecret := func(path string) string {
		if path == "" {
			return ""
		}
		out, err := exec.Command("op", "read", path).Output()
		if err != nil {
			logger.Config("op read failed for %s: %v", path, err)
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	apiToken = os.Getenv("JIRA_API_TOKEN")
	if apiToken == "" && userConfig.OPJiraTokenPath != "" {
		apiToken = readSecret(userConfig.OPJiraTokenPath)
	}
	if apiToken == "" {
		return nil, errors.NewOnePasswordError()
	}
	// Validate token if possible
	if !isJiraTokenValid(userConfig.JiraURL, email, apiToken) {
		logger.Config("API token validation failed, proceeding anyway")
	}

	// Determine projects using user config
	var projects []string
	if projectFlag == "both" {
		projects = userConfig.Projects
	} else {
		// Validate that the selected project is in our available list
		availableProjects := usercfg.GetAvailableProjectsFromRuntime()
		validProject := false
		for _, availableProj := range availableProjects {
			if projectFlag == availableProj && availableProj != "both" {
				validProject = true
				break
			}
		}
		if !validProject {
			return nil, errors.NewInvalidProjectError(projectFlag, availableProjects)
		}
		projects = []string{projectFlag}
	}

	return &Config{
		JiraURL:         userConfig.JiraURL,
		Email:           email,
		APIToken:        apiToken,
		Projects:        projects,
		All:             allFlag,
		EnableClaude:    userConfig.ClaudeEnabled(),
		EnableWorktrees: userConfig.WorktreesEnabled(),
	}, nil
}

// isJiraTokenValid checks if the given email/token can authenticate to Jira by calling /myself
func isJiraTokenValid(jiraURL, email, token string) bool {
	if jiraURL == "" || email == "" || token == "" {
		return false
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	client := httputil.NewRetryableClient(5*time.Second, 1) // Quick validation, minimal retries
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/myself", jiraURL), nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// fetchJiraEmail calls /rest/api/3/myself and returns the account's email address.
func fetchJiraEmail(jiraURL, authEmail, token string) (string, error) {
	if jiraURL == "" || authEmail == "" || token == "" {
		return "", fmt.Errorf("missing credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := httputil.NewRetryableClient(5*time.Second, 1)
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/myself", jiraURL), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(authEmail, token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("JIRA API returned %d", resp.StatusCode)
	}

	var result struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.EmailAddress, nil
}

func fetchIssues(config *Config) ([]JiraIssue, error) {
	// Build project filter
	var projectFilter string
	if len(config.Projects) == 1 {
		projectFilter = fmt.Sprintf("project = %s", config.Projects[0])
	} else {
		projectFilter = fmt.Sprintf("project in (%s)", strings.Join(config.Projects, ", "))
	}

	// Build JQL query
	var jql string
	if config.All {
		jql = fmt.Sprintf("%s AND (status = Open OR status = \"In Progress\" OR status = \"Change Approved\") ORDER BY created", projectFilter)
	} else {
		jql = fmt.Sprintf("%s AND (status = Open OR status = \"In Progress\" OR status = \"Change Approved\") AND reporter in (currentUser()) ORDER BY created", projectFilter)
	}

	// Make HTTP request with context and retry
	ctx, cancel := context.WithTimeout(context.Background(), httputil.DefaultTimeout)
	defer cancel()
	
	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/search/jql", config.JiraURL), nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Accept", "application/json")

	q := req.URL.Query()
	q.Add("jql", jql)
	q.Add("maxResults", "10")
	q.Add("fields", getFieldsList())
	req.URL.RawQuery = q.Encode()

	var jiraResp JiraResponse
	if err := client.DoJSONRequest(ctx, req, &jiraResp); err != nil {
		return nil, errors.WrapWithContext(err, "jira_connection")
	}

	return jiraResp.Issues, nil
}

func selectIssue(issues []JiraIssue) (JiraIssue, error) {
	var options []string
	for _, issue := range issues {
		options = append(options, fmt.Sprintf("%s: %s", issue.Key, issue.Fields.Summary))
	}

	prompt := &survey.Select{
		Message: "Select an issue to create a branch for:",
		Options: options,
	}

	var selectedIndex int
	if err := survey.AskOne(prompt, &selectedIndex); err != nil {
		return JiraIssue{}, err
	}

	return issues[selectedIndex], nil
}

func createBranchName(issue JiraIssue) string {
	return makeBranchName(issue.Key, issue.Fields.Summary)
}

// makeBranchName creates a branch name from a JIRA key and summary string
func makeBranchName(key, summary string) string {
	summary = strings.ToLower(summary)
	// Replace non-alphanumeric with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	summary = reg.ReplaceAllString(summary, "-")
	summary = strings.Trim(summary, "-")
	// Truncate to reasonable length
	if len(summary) > 50 {
		summary = summary[:50]
		summary = strings.TrimRight(summary, "-")
	}
	return fmt.Sprintf("%s_%s", key, summary)
}

func createOrCheckoutWorktree(branchName string) WorktreeResult {
	// Get repository root
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOutput, err := rootCmd.Output()
	if err != nil {
		return WorktreeResult{Error: fmt.Errorf("not in a git repository: %w", err)}
	}
	repoRoot := strings.TrimSpace(string(rootOutput))
	repoName := filepath.Base(repoRoot)

	// Sibling directory: ../repo-BRANCH
	parentDir := filepath.Dir(repoRoot)
	worktreePath := filepath.Join(parentDir, fmt.Sprintf("%s-%s", repoName, branchName))

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return WorktreeResult{
			Path:       worktreePath,
			BranchName: branchName,
			Created:    false,
		}
	}

	// Check if branch exists
	checkCmd := exec.Command("git", "rev-parse", "--verify", branchName)
	branchExists := checkCmd.Run() == nil

	var createCmd *exec.Cmd
	if branchExists {
		createCmd = exec.Command("git", "worktree", "add", worktreePath, branchName)
	} else {
		createCmd = exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
	}

	var stderr bytes.Buffer
	createCmd.Stderr = &stderr

	if err := createCmd.Run(); err != nil {
		return WorktreeResult{Error: fmt.Errorf("worktree creation failed: %s", stderr.String())}
	}

	return WorktreeResult{
		Path:       worktreePath,
		BranchName: branchName,
		Created:    true,
	}
}

func extractDescriptionText(issue JiraIssue) string {
	if issue.Fields.Description == nil {
		return ""
	}
	var texts []string
	for _, block := range issue.Fields.Description.Content {
		for _, inline := range block.Content {
			if inline.Text != "" {
				texts = append(texts, inline.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

func spawnClaudeWithContext(worktreePath string, issue JiraIssue) error {
	description := extractDescriptionText(issue)
	prompt := fmt.Sprintf("Working on %s: %s\n\n%s",
		issue.Key,
		issue.Fields.Summary,
		description)

	cmd := exec.Command("claude", prompt)
	cmd.Dir = worktreePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func createOrCheckoutBranch(branchName string) error {
	// Check for uncommitted changes that would block checkout
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusOut, _ := statusCmd.Output()
	if len(strings.TrimSpace(string(statusOut))) > 0 {
		fmt.Printf("\033[93mYou have uncommitted changes.\033[0m\n")
		var doStash bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "Stash changes and continue?",
			Default: true,
		}, &doStash); err != nil || !doStash {
			return fmt.Errorf("branch switch cancelled: uncommitted changes")
		}
		stashCmd := exec.Command("git", "stash", "push", "-m", fmt.Sprintf("gci: auto-stash before switching to %s", branchName))
		if out, err := stashCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git stash failed: %s", strings.TrimSpace(string(out)))
		}
		fmt.Printf("\033[92mChanges stashed.\033[0m\n")
	}

	// Check if branch exists
	checkCmd := exec.Command("git", "rev-parse", "--verify", branchName)
	if err := checkCmd.Run(); err == nil {
		// Branch exists, checkout
		fmt.Printf("\033[92mBranch \"%s\" already exists. Checking out the branch.\033[0m\n", branchName)
		checkoutCmd := exec.Command("git", "checkout", branchName)
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	} else {
		// Branch doesn't exist, create and checkout
		fmt.Printf("\033[92mCreating and checking out branch \"%s\".\033[0m\n", branchName)
		createCmd := exec.Command("git", "checkout", "-b", branchName)
		if out, err := createCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout -b failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
}

// openIssueInBrowser opens the selected issue in the default browser
func openIssueInBrowser(config *Config, issue JiraIssue) error {
	url := fmt.Sprintf("%s/browse/%s", config.JiraURL, issue.Key)
	return browser.OpenURL(url)
}

// ---- gci create: retroactive ticket creation ----

// ticketSuggestion holds the AI-generated title and description for a new ticket
type ticketSuggestion struct {
	Title       string
	Description string
}

// JIRA issue creation request/response types
type createIssueRequest struct {
	Fields createIssueFields `json:"fields"`
}

type createIssueFields struct {
	Project   projectRef   `json:"project"`
	Summary   string       `json:"summary"`
	IssueType issueTypeRef `json:"issuetype"`
	Assignee  *assigneeRef `json:"assignee,omitempty"`
	Description *adfDocument `json:"description,omitempty"`
}

type projectRef struct {
	Key string `json:"key"`
}

type issueTypeRef struct {
	Name string `json:"name"`
}

type assigneeRef struct {
	AccountID string `json:"accountId"`
}

type adfDocument struct {
	Type    string     `json:"type"`
	Version int        `json:"version"`
	Content []adfBlock `json:"content"`
}

type adfBlock struct {
	Type    string      `json:"type"`
	Content []adfInline `json:"content,omitempty"`
}

type adfInline struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type createIssueResponse struct {
	Key  string `json:"key"`
	Self string `json:"self"`
}

// getCurrentBranch returns the current git branch name
func getCurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isProtectedBranch returns true for branches that should not be renamed
func isProtectedBranch(branch string) bool {
	switch branch {
	case "main", "master", "develop", "HEAD":
		return true
	default:
		return false
	}
}

// captureGitDiff auto-detects and captures the relevant diff for ticket generation
func captureGitDiff() (string, error) {
	var diffParts []string

	// 1. Check for uncommitted changes (staged + unstaged)
	cmd := exec.Command("git", "diff", "HEAD")
	out, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		diffParts = append(diffParts, string(out))
	}

	// 2. If no uncommitted changes, get commits since main
	if len(diffParts) == 0 {
		cmd = exec.Command("git", "diff", "main...HEAD")
		out, err = cmd.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			diffParts = append(diffParts, string(out))
		}
	}

	// 3. Append untracked file names
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	out, err = cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		diffParts = append(diffParts, "Untracked files:\n"+string(out))
	}

	if len(diffParts) == 0 {
		return "", fmt.Errorf("no changes detected (clean tree with no branch commits)")
	}

	result := strings.Join(diffParts, "\n")

	// Truncate to 8000 chars if needed
	if len(result) > 8000 {
		result = result[:8000] + "\n... [truncated]"
	}

	return result, nil
}

// renameBranch renames the current branch to newName
func renameBranch(newName string) error {
	cmd := exec.Command("git", "branch", "-m", newName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("branch rename failed: %s", stderr.String())
	}
	return nil
}

// generateTicketSuggestion uses Claude to analyze the diff and suggest a ticket
func generateTicketSuggestion(diff string, model string) (ticketSuggestion, error) {
	// Check if claude is available
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Println("\033[93mclaude not found in PATH — falling back to manual entry\033[0m")
		return manualTicketEntry()
	}

	prompt := fmt.Sprintf(`Analyze this git diff and suggest a JIRA ticket. Reply with exactly two lines:
TITLE: <a short imperative title for the ticket, max 80 chars>
DESCRIPTION: <a one-sentence description of what the change does>

Do not include any other text, markdown, or formatting. Just the two lines.

%s`, diff)

	args := []string{"-p", prompt}
	if model != "" {
		args = append([]string{"--model", model}, args...)
	}
	cmd := exec.Command("claude", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("\033[93mClaude failed (%v) — falling back to manual entry\033[0m\n", err)
		return manualTicketEntry()
	}

	suggestion, err := parseTicketSuggestion(stdout.String())
	if err != nil {
		fmt.Printf("\033[93mCould not parse Claude output — falling back to manual entry\033[0m\n")
		fmt.Printf("Raw output:\n%s\n", stdout.String())
		return manualTicketEntry()
	}

	return suggestion, nil
}

// parseTicketSuggestion extracts title and description from Claude's output
func parseTicketSuggestion(output string) (ticketSuggestion, error) {
	var s ticketSuggestion
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TITLE:") {
			s.Title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
		} else if strings.HasPrefix(line, "DESCRIPTION:") {
			s.Description = strings.TrimSpace(strings.TrimPrefix(line, "DESCRIPTION:"))
		}
	}
	if s.Title == "" {
		return s, fmt.Errorf("no TITLE found in output")
	}
	if s.Description == "" {
		return s, fmt.Errorf("no DESCRIPTION found in output")
	}
	return s, nil
}

// manualTicketEntry prompts the user to type title and description manually
func manualTicketEntry() (ticketSuggestion, error) {
	var s ticketSuggestion
	if err := survey.AskOne(&survey.Input{Message: "Ticket title:"}, &s.Title, survey.WithValidator(survey.Required)); err != nil {
		return s, err
	}
	if err := survey.AskOne(&survey.Input{Message: "Ticket description:"}, &s.Description, survey.WithValidator(survey.Required)); err != nil {
		return s, err
	}
	return s, nil
}

// confirmTicketDetails displays the suggestion and lets the user edit or accept it
func confirmTicketDetails(suggestion ticketSuggestion) (string, string, error) {
	fmt.Printf("\n  Title:       %s\n", suggestion.Title)
	fmt.Printf("  Description: %s\n\n", suggestion.Description)

	var choice string
	if err := survey.AskOne(&survey.Select{
		Message: "Use this suggestion?",
		Options: []string{"Use as-is", "Edit title", "Edit both", "Cancel"},
	}, &choice); err != nil {
		return "", "", err
	}

	title := suggestion.Title
	description := suggestion.Description

	switch choice {
	case "Use as-is":
		// keep as-is
	case "Edit title":
		if err := survey.AskOne(&survey.Input{Message: "Title:", Default: title}, &title); err != nil {
			return "", "", err
		}
	case "Edit both":
		if err := survey.AskOne(&survey.Input{Message: "Title:", Default: title}, &title); err != nil {
			return "", "", err
		}
		if err := survey.AskOne(&survey.Input{Message: "Description:", Default: description}, &description); err != nil {
			return "", "", err
		}
	case "Cancel":
		return "", "", fmt.Errorf("cancelled by user")
	}

	return title, description, nil
}

// resolveTargetProject determines which JIRA project to use
func resolveTargetProject(config *Config) (string, error) {
	// Flag takes priority
	if createProjectFlag != "" {
		return createProjectFlag, nil
	}

	// Single project — use it
	if len(config.Projects) == 1 {
		return config.Projects[0], nil
	}

	// Multiple projects — prompt
	var project string
	if err := survey.AskOne(&survey.Select{
		Message: "Which project?",
		Options: config.Projects,
	}, &project); err != nil {
		return "", err
	}
	return project, nil
}

// getMyAccountId fetches the current user's JIRA account ID
func getMyAccountId(config *Config) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httputil.DefaultTimeout)
	defer cancel()

	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/myself", config.JiraURL), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Accept", "application/json")

	var result struct {
		AccountID string `json:"accountId"`
	}
	if err := client.DoJSONRequest(ctx, req, &result); err != nil {
		return "", fmt.Errorf("failed to fetch JIRA account: %w", err)
	}
	return result.AccountID, nil
}

// createJiraIssue creates a new JIRA issue and returns the issue key
func createJiraIssue(config *Config, project, title, description, issueType, accountId string) (string, error) {
	// Build ADF description
	var desc *adfDocument
	if description != "" {
		desc = &adfDocument{
			Type:    "doc",
			Version: 1,
			Content: []adfBlock{
				{
					Type: "paragraph",
					Content: []adfInline{
						{Type: "text", Text: description},
					},
				},
			},
		}
	}

	body := createIssueRequest{
		Fields: createIssueFields{
			Project:     projectRef{Key: project},
			Summary:     title,
			IssueType:   issueTypeRef{Name: issueType},
			Assignee:    &assigneeRef{AccountID: accountId},
			Description: desc,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), httputil.DefaultTimeout)
	defer cancel()

	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/rest/api/3/issue", config.JiraURL), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Use DoWithRetry directly since JIRA returns 201 (not 200) on success
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("JIRA request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("JIRA returned %d: %s", resp.StatusCode, string(respBody))
	}

	var issueResp createIssueResponse
	if err := json.Unmarshal(respBody, &issueResp); err != nil {
		return "", fmt.Errorf("failed to parse JIRA response: %w", err)
	}

	return issueResp.Key, nil
}

// runCreate is the orchestrator for the `gci create` command
func runCreate(cmd *cobra.Command, args []string) {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	currentBranch := getCurrentBranch()
	onProtected := isProtectedBranch(currentBranch)

	// Capture changes
	fmt.Println("Capturing changes...")
	diff, err := captureGitDiff()
	if err != nil {
		fmt.Printf("\033[93m%v\033[0m\n", err)
		return
	}

	// Show diff stats
	statCmd := exec.Command("git", "diff", "--stat", "HEAD")
	if statOut, err := statCmd.Output(); err == nil && len(strings.TrimSpace(string(statOut))) > 0 {
		fmt.Printf("  %s\n", strings.TrimSpace(string(statOut)))
	}

	// Start ticket suggestion (Claude in background if enabled, otherwise manual entry after project selection)
	type suggestionResult struct {
		suggestion ticketSuggestion
		err        error
	}
	var suggCh chan suggestionResult
	if config.EnableClaude {
		suggCh = make(chan suggestionResult, 1)
		go func() {
			s, err := generateTicketSuggestion(diff, createModel)
			suggCh <- suggestionResult{s, err}
		}()
	}

	// Resolve project (user prompt runs concurrently with Claude when enabled)
	project, err := resolveTargetProject(config)
	if err != nil {
		fmt.Println("\n\033[93mOperation cancelled by user.\033[0m")
		return
	}

	// Get ticket suggestion
	var suggResult suggestionResult
	if config.EnableClaude {
		fmt.Println("\nGenerating ticket suggestion...")
		suggResult = <-suggCh
	} else {
		s, err := manualTicketEntry()
		suggResult = suggestionResult{s, err}
	}
	if suggResult.err != nil {
		fmt.Println("\n\033[93mOperation cancelled by user.\033[0m")
		return
	}
	suggestion := suggResult.suggestion

	// Confirm with user
	title, description, err := confirmTicketDetails(suggestion)
	if err != nil {
		fmt.Println("\n\033[93mOperation cancelled by user.\033[0m")
		return
	}

	// Dry-run: print summary and exit
	if createDryRun {
		fmt.Println("\n\033[96m[dry-run] Would create:\033[0m")
		fmt.Printf("  Project:     %s\n", project)
		fmt.Printf("  Type:        %s\n", createIssueType)
		fmt.Printf("  Title:       %s\n", title)
		fmt.Printf("  Description: %s\n", description)
		branchPreview := makeBranchName(project+"-???", title)
		fmt.Printf("  Branch:      %s\n", branchPreview)
		return
	}

	// Create the ticket
	fmt.Print("Creating ticket... ")
	accountId, err := getMyAccountId(config)
	if err != nil {
		log.Fatalf("Failed to get JIRA account: %v", err)
	}

	issueKey, err := createJiraIssue(config, project, title, description, createIssueType, accountId)
	if err != nil {
		log.Fatalf("Failed to create JIRA issue: %v", err)
	}
	fmt.Printf("\033[92m%s\033[0m\n", issueKey)

	// Branch rename
	newBranch := makeBranchName(issueKey, title)
	if !createNoRename {
		if onProtected {
			fmt.Printf("On protected branch %q — creating new branch %q\n", currentBranch, newBranch)
			if err := createOrCheckoutBranch(newBranch); err != nil {
				fmt.Printf("\033[91mFailed to create branch: %v\033[0m\n", err)
				fmt.Println("You can rename manually with: git checkout -b", newBranch)
			}
		} else {
			fmt.Printf("Renaming branch... %s -> %s\n", currentBranch, newBranch)
			if err := renameBranch(newBranch); err != nil {
				fmt.Printf("\033[91m%v\033[0m\n", err)
				fmt.Println("You can rename manually with: git branch -m", newBranch)
			}
		}
	}

	fmt.Printf("\nView: %s/browse/%s\n", config.JiraURL, issueKey)
}

// ---- TUI: Personal Kanban ----

// We implement a minimal TUI using Bubble Tea to list three columns of issues.
// To keep the code self-contained, the Kanban TUI is defined here in the main package.

type scopeFilter int

const (
	scopeMineOrReported scopeFilter = iota // assigned to me OR reported by me
	scopeMine                              // assigned to me
	scopeReported                          // reported by me
	scopeUnassigned                        // unassigned in team backlog
)

// kanbanColumn represents a logical column backed by a JQL filter on statusCategory
type kanbanColumn struct {
	title          string
	statusCategory string // "To Do", "In Progress", "Done"
	issues         []JiraIssue
}

// buildProjectFilter creates the JQL project predicate
func buildProjectFilter(projects []string) string {
	if len(projects) == 1 {
		return fmt.Sprintf("project = %s", projects[0])
	}
	return fmt.Sprintf("project in (%s)", strings.Join(projects, ", "))
}

func buildScopePredicate(scope scopeFilter) string {
	switch scope {
	case scopeMineOrReported:
		return "(assignee = currentUser() OR reporter = currentUser())"
	case scopeMine:
		return "assignee = currentUser()"
	case scopeReported:
		return "reporter = currentUser()"
	case scopeUnassigned:
		return "assignee is EMPTY"
	default:
		return ""
	}
}

// getFieldsList returns the appropriate fields list based on UI preferences
func getFieldsList() string {
	fields := "summary,project,issuetype,parent,status"
	uiPrefs := usercfg.GetUIPrefs()
	if uiPrefs.ShowExtraFields {
		// Add assignee and priority for extra fields display
		fields += ",assignee,priority"
	}
	return fields
}

// fetchColumnIssues fetches up to maxResults issues for a given statusCategory + scope
func fetchColumnIssues(config *Config, statusCategory string, scope scopeFilter, maxResults int) ([]JiraIssue, error) {
	projectFilter := buildProjectFilter(config.Projects)
	scopePredicate := buildScopePredicate(scope)

	var predicates []string
	predicates = append(predicates, projectFilter)
	predicates = append(predicates, fmt.Sprintf("statusCategory = \"%s\"", statusCategory))
	if scopePredicate != "" {
		predicates = append(predicates, scopePredicate)
	}
	jql := strings.Join(predicates, " AND ") + " ORDER BY updated DESC"

	ctx, cancel := context.WithTimeout(context.Background(), httputil.DefaultTimeout)
	defer cancel()
	
	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/search/jql", config.JiraURL), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Add("jql", jql)
	q.Add("maxResults", fmt.Sprintf("%d", maxResults))
	q.Add("fields", getFieldsList())
	req.URL.RawQuery = q.Encode()

	logger.HTTP("GET", req.URL.String())
	
	var jiraResp JiraResponse
	if err := client.DoJSONRequest(ctx, req, &jiraResp); err != nil {
		logger.JIRA("request failed: %v", err)
		return nil, errors.WrapWithContext(err, "jira_connection")
	}
	
	logger.JIRA("Fetched %d issues for statusCategory=%q scope=%q", len(jiraResp.Issues), statusCategory, scopeToString(scope))
	return jiraResp.Issues, nil
}

// fetchColumnIssuesWithContext fetches column issues with a provided context for cancellation
func fetchColumnIssuesWithContext(ctx context.Context, config *Config, statusCategory string, scope scopeFilter, maxResults int) ([]JiraIssue, error) {
	projectFilter := buildProjectFilter(config.Projects)
	scopePredicate := buildScopePredicate(scope)

	var predicates []string
	predicates = append(predicates, projectFilter)
	predicates = append(predicates, fmt.Sprintf("statusCategory = \"%s\"", statusCategory))
	if scopePredicate != "" {
		predicates = append(predicates, scopePredicate)
	}
	jql := strings.Join(predicates, " AND ") + " ORDER BY updated DESC"
	
	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/search/jql", config.JiraURL), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Add("jql", jql)
	q.Add("maxResults", fmt.Sprintf("%d", maxResults))
	q.Add("fields", getFieldsList())
	req.URL.RawQuery = q.Encode()

	logger.HTTP("GET", req.URL.String())
	
	var jiraResp JiraResponse
	if err := client.DoJSONRequest(ctx, req, &jiraResp); err != nil {
		logger.JIRA("request failed: %v", err)
		return nil, errors.WrapWithContext(err, "jira_connection")
	}
	
	logger.JIRA("Fetched %d issues for statusCategory=%q scope=%q", len(jiraResp.Issues), statusCategory, scopeToString(scope))
	return jiraResp.Issues, nil
}

// fetchIssuesWithJQL fetches issues using a custom JQL query
func fetchIssuesWithJQL(config *Config, jql string, maxResults int) ([]JiraIssue, error) {
	// Inject project filter into custom JQL if it doesn't already specify projects
	if !strings.Contains(strings.ToLower(jql), "project") {
		projectFilter := buildProjectFilter(config.Projects)
		jql = projectFilter + " AND (" + jql + ")"
	}

	ctx, cancel := context.WithTimeout(context.Background(), httputil.DefaultTimeout)
	defer cancel()
	
	client := httputil.NewDefaultClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/3/search/jql", config.JiraURL), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(config.Email, config.APIToken)
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Add("jql", jql)
	q.Add("maxResults", fmt.Sprintf("%d", maxResults))
	q.Add("fields", getFieldsList())
	req.URL.RawQuery = q.Encode()

	logger.HTTP("GET", req.URL.String())
	
	var jiraResp JiraResponse
	if err := client.DoJSONRequest(ctx, req, &jiraResp); err != nil {
		logger.JIRA("JQL request failed: %v", err)
		return nil, errors.WrapWithContext(err, "jira_connection")
	}
	
	return jiraResp.Issues, nil
}

// runBoard launches the TUI. We implement a very small in-terminal navigable board with columns.
func runBoard(cmd *cobra.Command, args []string) {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := StartBoard(config); err != nil {
		log.Fatalf("Board failed: %v", err)
	}
}

func runSetup(cmd *cobra.Command, args []string) {
	fmt.Println("GCI Setup Wizard")
	fmt.Println("=================")

	currentConfig := usercfg.GetRuntimeConfig()
	newConfig := currentConfig
	isFirstRun := !usercfg.IsConfigured()

	if isFirstRun {
		fmt.Println("Welcome! Let's configure GCI for your environment.")
		fmt.Println()
	} else {
		fmt.Printf("Existing config found at %s — modifying.\n\n", usercfg.Path())
		fmt.Printf("  JIRA URL: %s\n", currentConfig.JiraURL)
		fmt.Printf("  Projects: %v\n", currentConfig.Projects)
		fmt.Printf("  Default Scope: %s\n", currentConfig.DefaultScope)
		fmt.Printf("  Boards: %v\n", currentConfig.Boards)
		fmt.Printf("  Claude AI: %v\n", currentConfig.ClaudeEnabled())
		fmt.Printf("  Worktrees: %v\n", currentConfig.WorktreesEnabled())
		fmt.Println()
	}

	// JIRA URL (always prompt on first run)
	if isFirstRun || currentConfig.JiraURL == "" {
		var jiraURL string
		if err := survey.AskOne(&survey.Input{
			Message: "JIRA URL (e.g. https://your-company.atlassian.net):",
			Default: currentConfig.JiraURL,
		}, &jiraURL, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
		newConfig.JiraURL = jiraURL
	}

	// Projects
	setupProjects := isFirstRun
	if !isFirstRun {
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Change projects? (currently: %s)", strings.Join(currentConfig.Projects, ", ")),
			Default: false,
		}, &setupProjects); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
	}

	if setupProjects {
		var projectInput string
		defaultVal := strings.Join(currentConfig.Projects, ", ")
		if err := survey.AskOne(&survey.Input{
			Message: "Project keys (comma-separated, e.g. PROJ,INFRA):",
			Default: defaultVal,
		}, &projectInput, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
		projects := strings.Split(projectInput, ",")
		var cleaned []string
		for _, p := range projects {
			p = strings.TrimSpace(p)
			if p != "" {
				cleaned = append(cleaned, strings.ToUpper(p))
			}
		}
		if len(cleaned) > 0 {
			newConfig.Projects = cleaned
		}
	}

	// Scope
	setupScope := isFirstRun
	if !isFirstRun {
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Change default scope? (currently: %s)", currentConfig.DefaultScope),
			Default: false,
		}, &setupScope); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
	}

	if setupScope {
		scopeOptions := []string{"assigned_or_reported (default)", "assigned", "reported", "unassigned"}
		scopeDefault := currentConfig.DefaultScope
		if scopeDefault == "" || scopeDefault == "assigned_or_reported" {
			scopeDefault = "assigned_or_reported (default)"
		}
		var scopeSelection string
		if err := survey.AskOne(&survey.Select{
			Message: "Which issues should appear by default?",
			Options: scopeOptions,
			Default: scopeDefault,
		}, &scopeSelection); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
		// Strip display suffix before saving
		newConfig.DefaultScope = strings.TrimSuffix(scopeSelection, " (default)")
	}

	// 1Password setup
	var configureOP bool
	if !isFirstRun {
		if err := survey.AskOne(&survey.Confirm{
			Message: "Change 1Password settings?",
			Default: false,
		}, &configureOP); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
	} else {
		if err := survey.AskOne(&survey.Confirm{
			Message: "Use 1Password for API tokens?",
			Default: true,
		}, &configureOP); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
	}

	// Warn if op CLI is not installed but user wants 1Password
	if configureOP {
		if _, err := exec.LookPath("op"); err != nil {
			fmt.Println()
			fmt.Println("  Warning: 1Password CLI (op) is not installed.")
			fmt.Println("  Install it from: https://developer.1password.com/docs/cli/get-started/")
			fmt.Println()
			fmt.Println("  You can still configure 1Password now and install the CLI later,")
			fmt.Println("  or skip this step and use the JIRA_API_TOKEN environment variable instead.")
			fmt.Println()

			var continueOP bool
			if err := survey.AskOne(&survey.Confirm{
				Message: "Continue with 1Password setup anyway?",
				Default: false,
			}, &continueOP); err != nil {
				fmt.Println("Setup cancelled")
				return
			}
			if !continueOP {
				fmt.Println()
				fmt.Println("  Skipped 1Password setup.")
				fmt.Println("  Set JIRA_API_TOKEN as an environment variable to authenticate.")
				configureOP = false
			}
		}
	}

	if configureOP {
		fmt.Println()
		fmt.Println("  To store your JIRA API token in 1Password:")
		fmt.Println("    1. Create an API token at: https://id.atlassian.com/manage-profile/security/api-tokens")
		fmt.Println("       (click your avatar top-right to check which email is used)")
		fmt.Println("    2. In 1Password, create a new item (type: API Credential)")
		fmt.Println("    3. Set the username to your Atlassian email (e.g. you@company.com)")
		fmt.Println("    4. Paste the API token as the credential")
		fmt.Println()

		// Extract existing item name from op:// path if re-running
		existingJiraItem := ""
		if currentConfig.OPJiraTokenPath != "" {
			parts := strings.Split(currentConfig.OPJiraTokenPath, "/")
			if len(parts) >= 4 {
				existingJiraItem = parts[3]
			}
		}

		var jiraItemName string
		if err := survey.AskOne(&survey.Input{
			Message: "1Password item name for JIRA API token:",
			Default: existingJiraItem,
		}, &jiraItemName, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println("Setup cancelled")
			return
		}
		newConfig.OPJiraTokenPath = fmt.Sprintf("op://Private/%s/credential", jiraItemName)

	}

	// Claude AI integration
	claudeDefault := currentConfig.ClaudeEnabled()
	if isFirstRun {
		// Auto-detect Claude in PATH for first-run default
		if _, err := exec.LookPath("claude"); err == nil {
			claudeDefault = true
		}
	}
	var enableClaude bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Enable Claude AI integration?",
		Default: claudeDefault,
	}, &enableClaude); err != nil {
		fmt.Println("Setup cancelled")
		return
	}
	newConfig.EnableClaude = &enableClaude

	// Git worktrees for Interactive Mode
	worktreeDefault := currentConfig.WorktreesEnabled()
	var enableWorktrees bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Enable git worktrees for Interactive Mode?",
		Default: worktreeDefault,
	}, &enableWorktrees); err != nil {
		fmt.Println("Setup cancelled")
		return
	}
	newConfig.EnableWorktrees = &enableWorktrees

	// Save config before auth-dependent steps so loadConfig() can find it
	if err := usercfg.Save(newConfig); err != nil {
		log.Fatalf("Failed to save configuration: %v", err)
	}

	// Resolve auth inline for email detection and board discovery.
	// We do this directly instead of loadConfig() to avoid its os.Exit guard
	// and to handle the email mismatch case before anything depends on it.
	var authEmail, apiToken string
	var authOK bool

	// Get git email for comparison
	var gitEmail string
	if gitEmailOut, err := exec.Command("git", "config", "user.email").Output(); err == nil {
		gitEmail = strings.TrimSpace(string(gitEmailOut))
	}

	// Resolve API token: env var > 1Password
	apiToken = os.Getenv("JIRA_API_TOKEN")
	if apiToken == "" && newConfig.OPJiraTokenPath != "" {
		fmt.Println("\nVerifying JIRA authentication via 1Password...")
		if out, err := exec.Command("op", "read", newConfig.OPJiraTokenPath).Output(); err == nil {
			apiToken = strings.TrimSpace(string(out))
		}
	}

	// Resolve JIRA email: prefer 1Password username, fall back to git email + /myself
	if newConfig.OPJiraTokenPath != "" {
		// Derive username path from credential path: op://Private/<item>/credential → op://Private/<item>/username
		usernamePath := strings.TrimSuffix(newConfig.OPJiraTokenPath, "/credential") + "/username"
		if out, err := exec.Command("op", "read", usernamePath).Output(); err == nil {
			opEmail := strings.TrimSpace(string(out))
			if opEmail != "" {
				authEmail = opEmail

				// Auto-create domain mapping if git email domain differs
				if gitEmail != "" {
					gitParts := strings.SplitN(gitEmail, "@", 2)
					opParts := strings.SplitN(opEmail, "@", 2)
					if len(gitParts) == 2 && len(opParts) == 2 && gitParts[1] != opParts[1] {
						if newConfig.EmailDomainMap == nil {
							newConfig.EmailDomainMap = make(map[string]string)
						}
						newConfig.EmailDomainMap[gitParts[1]] = opParts[1]
						fmt.Printf("\nGit email (%s) differs from JIRA email (%s).\n", gitEmail, opEmail)
						fmt.Printf("Added email domain mapping: %s → %s\n", gitParts[1], opParts[1])
					}
				}
			}
		}
	}

	// Fall back to git email if 1Password username wasn't available
	if authEmail == "" && gitEmail != "" {
		authEmail = gitEmail
	}

	if authEmail != "" && apiToken != "" {
		// Verify auth works
		if _, err := fetchJiraEmail(newConfig.JiraURL, authEmail, apiToken); err == nil {
			authOK = true
		} else {
			// Auth failed — ask for JIRA email
			fmt.Printf("\nCould not authenticate to JIRA with %s.\n", authEmail)
			var jiraEmailInput string
			if err := survey.AskOne(&survey.Input{
				Message: "What email do you use to log in to JIRA?",
			}, &jiraEmailInput, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println("Setup cancelled")
				return
			}
			jiraEmailInput = strings.TrimSpace(jiraEmailInput)

			// Verify the provided email works
			if _, verifyErr := fetchJiraEmail(newConfig.JiraURL, jiraEmailInput, apiToken); verifyErr == nil {
				authOK = true
				// Auto-create domain mapping if domains differ
				if gitEmail != "" {
					gitParts := strings.SplitN(gitEmail, "@", 2)
					jiraParts := strings.SplitN(jiraEmailInput, "@", 2)
					if len(gitParts) == 2 && len(jiraParts) == 2 && gitParts[1] != jiraParts[1] {
						if newConfig.EmailDomainMap == nil {
							newConfig.EmailDomainMap = make(map[string]string)
						}
						newConfig.EmailDomainMap[gitParts[1]] = jiraParts[1]
						fmt.Printf("Added email domain mapping: %s → %s\n", gitParts[1], jiraParts[1])
					}
				}
				authEmail = jiraEmailInput
			} else {
				fmt.Println("Warning: Could not authenticate with that email either.")
			}
		}
	}

	// Save again if email detection added a domain mapping
	if err := usercfg.Save(newConfig); err != nil {
		log.Fatalf("Failed to save configuration: %v", err)
	}

	// Board discovery — automatic when auth is available
	if authOK {
		fmt.Println("\nDiscovering project boards from JIRA...")
		boards, err := jira.DiscoverBoards(newConfig.JiraURL, authEmail, apiToken, newConfig.Projects...)
		if err != nil {
			fmt.Printf("Warning: Board discovery failed: %v\n", err)
		} else {
			rankedBoards := jira.RankBoards(boards, newConfig.Projects)

			if len(rankedBoards) > 0 {
				var boardOptions []string
				boardMap := make(map[string]jira.Board)

				for _, board := range rankedBoards[:min(10, len(rankedBoards))] {
					option := fmt.Sprintf("%s (ID: %d, Project: %s)", board.Name, board.ID, board.Location.ProjectKey)
					boardOptions = append(boardOptions, option)
					boardMap[option] = board
				}

				var selectedBoards []string
				if err := survey.AskOne(&survey.MultiSelect{
					Message: "Select your boards:",
					Options: boardOptions,
				}, &selectedBoards); err == nil {
					if newConfig.Boards == nil {
						newConfig.Boards = make(map[string]int)
					}
					for _, selected := range selectedBoards {
						if board, ok := boardMap[selected]; ok {
							key := fmt.Sprintf("%s_%s", board.Location.ProjectKey, strings.ToLower(board.Type))
							newConfig.Boards[key] = board.ID
						}
					}
				}
			}
		}
	}

	if err := usercfg.Save(newConfig); err != nil {
		log.Fatalf("Failed to save configuration: %v", err)
	}

	fmt.Printf("\nConfiguration saved to: %s\n", usercfg.Path())
	fmt.Println("\nFinal configuration:")
	fmt.Printf("  JIRA URL: %s\n", newConfig.JiraURL)
	fmt.Printf("  Projects: %v\n", newConfig.Projects)
	fmt.Printf("  Default Scope: %s\n", newConfig.DefaultScope)
	fmt.Printf("  Boards: %v\n", newConfig.Boards)
	fmt.Printf("  Claude AI: %v\n", newConfig.ClaudeEnabled())
	fmt.Printf("  Worktrees: %v\n", newConfig.WorktreesEnabled())
	if newConfig.OPJiraTokenPath != "" {
		fmt.Printf("  JIRA Token Path: %s\n", newConfig.OPJiraTokenPath)
	}
}

func runConfigMigrate(cmd *cobra.Command, args []string) {
	err := usercfg.MigrateAndSave()
	if err != nil {
		fmt.Printf("Migration failed: %v\n", err)
		os.Exit(1)
	}
}

func runConfigPath(cmd *cobra.Command, args []string) {
	fmt.Println(usercfg.Path())
}

func runConfigPrint(cmd *cobra.Command, args []string) {
	config := usercfg.GetRuntimeConfig()

	fmt.Printf("Configuration (effective):\n")
	fmt.Printf("  Schema Version: %d\n", config.SchemaVersion)
	fmt.Printf("  Projects: %v\n", config.Projects)
	fmt.Printf("  Default Scope: %s\n", config.DefaultScope)
	fmt.Printf("  JIRA URL: %s\n", config.JiraURL)
	fmt.Printf("  Boards: %v\n", config.Boards)
	fmt.Printf("  UI Preferences: %+v\n", config.UIPrefs)
	fmt.Printf("\nConfig file location: %s\n", usercfg.Path())
}

func runConfigGet(cmd *cobra.Command, args []string) {
	key := args[0]
	config := usercfg.GetRuntimeConfig()

	switch key {
	case "projects":
		for i, project := range config.Projects {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Print(project)
		}
		fmt.Println()
	case "default_scope":
		fmt.Println(config.DefaultScope)
	case "jira_url":
		fmt.Println(config.JiraURL)
	case "boards":
		first := true
		for name, id := range config.Boards {
			if !first {
				fmt.Print(",")
			}
			fmt.Printf("%s=%d", name, id)
			first = false
		}
		fmt.Println()
	case "schema_version":
		fmt.Println(config.SchemaVersion)
	default:
		fmt.Printf("Unknown key: %s\n", key)
		fmt.Println("Available keys: projects, default_scope, jira_url, boards, schema_version")
		os.Exit(1)
	}
}

func runConfigSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	// Load current config
	config, err := usercfg.Load()
	if err != nil && err != usercfg.ErrNotConfigured {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Validate and set the value
	switch key {
	case "default_scope":
		validScopes := []string{"assigned_or_reported", "assigned", "reported", "unassigned"}
		valid := false
		for _, scope := range validScopes {
			if value == scope {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Printf("Invalid scope: %s\n", value)
			fmt.Printf("Valid scopes: %s\n", strings.Join(validScopes, ", "))
			os.Exit(1)
		}
		config.DefaultScope = value

	case "jira_url":
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			fmt.Printf("Invalid JIRA URL: %s (must start with http:// or https://)\n", value)
			os.Exit(1)
		}
		config.JiraURL = value

	case "projects", "boards", "schema_version":
		fmt.Printf("Key '%s' cannot be set via 'config set'. Use 'gci setup' for projects and boards.\n", key)
		os.Exit(1)

	default:
		fmt.Printf("Unknown key: %s\n", key)
		fmt.Println("Settable keys: default_scope, jira_url")
		os.Exit(1)
	}

	// Save the updated config
	err = usercfg.Save(config)
	if err != nil {
		fmt.Printf("Failed to save config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Set %s = %s\n", key, value)
}

func runConfigDoctor(cmd *cobra.Command, args []string) {
	fmt.Println("🏥 GCI Configuration Doctor")
	fmt.Println("==========================")

	issues := 0

	// Check if config file exists
	configPath := usercfg.Path()
	legacyPath := usercfg.LegacyPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			fmt.Println("ℹ️  No config file found - using defaults")
			fmt.Printf("   Create one with: gci setup\n")
		} else {
			fmt.Println("⚠️  Using legacy config path")
			fmt.Printf("   Consider migrating: gci config migrate\n")
			fmt.Printf("   Legacy path: %s\n", legacyPath)
			fmt.Printf("   Preferred path: %s\n", configPath)
			issues++
		}
	} else {
		fmt.Println("✅ Config file found at XDG-compliant location")
	}

	// Load and validate config
	config := usercfg.GetRuntimeConfig()

	// Check schema version
	if config.SchemaVersion < usercfg.CurrentSchemaVersion {
		fmt.Printf("⚠️  Config schema is outdated (v%d, current: v%d)\n", config.SchemaVersion, usercfg.CurrentSchemaVersion)
		fmt.Println("   Run: gci config migrate")
		issues++
	} else {
		fmt.Printf("✅ Config schema is current (v%d)\n", config.SchemaVersion)
	}

	// Check projects
	if len(config.Projects) == 0 {
		fmt.Println("⚠️  No projects configured")
		fmt.Println("   Run: gci setup")
		issues++
	} else {
		fmt.Printf("✅ Projects configured: %v\n", config.Projects)
	}

	// Check default scope
	validScopes := []string{"assigned_or_reported", "assigned", "reported", "unassigned"}
	validScope := false
	for _, scope := range validScopes {
		if config.DefaultScope == scope {
			validScope = true
			break
		}
	}
	if !validScope {
		fmt.Printf("⚠️  Invalid default scope: %s\n", config.DefaultScope)
		fmt.Printf("   Valid scopes: %s\n", strings.Join(validScopes, ", "))
		issues++
	} else {
		fmt.Printf("✅ Default scope is valid: %s\n", config.DefaultScope)
	}

	// Check JIRA URL
	if config.JiraURL == "" {
		fmt.Println("⚠️  JIRA URL not configured")
		fmt.Println("   Run: gci setup")
		issues++
	} else if !strings.HasPrefix(config.JiraURL, "http://") && !strings.HasPrefix(config.JiraURL, "https://") {
		fmt.Printf("⚠️  Invalid JIRA URL format: %s\n", config.JiraURL)
		fmt.Println("   Must start with http:// or https://")
		issues++
	} else {
		fmt.Printf("✅ JIRA URL configured: %s\n", config.JiraURL)
	}

	fmt.Println()
	if issues == 0 {
		fmt.Println("🎉 No issues found! Configuration looks healthy.")
	} else {
		fmt.Printf("Found %d issue(s). See suggestions above.\n", issues)
		os.Exit(1)
	}
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println(version.GetVersionString())

	// Check for available updates (synchronous since user is asking about version)
	ch := version.StartUpdateCheck()
	select {
	case result := <-ch:
		if result.NewVersion != "" {
			fmt.Printf("\n\033[33mUpdate available: %s (current: %s)\033[0m\n", result.NewVersion, version.GetShortVersion())
			fmt.Println("\033[33mRun 'gci update' to upgrade.\033[0m")
		}
	case <-time.After(5 * time.Second):
		// Don't block forever if GitHub is slow
	}
}

func runUpdate(cmd *cobra.Command, args []string) {
	current := version.GetShortVersion()
	if current == "dev" {
		fmt.Println("Cannot self-update a dev build. Install a released version first.")
		return
	}

	source, err := version.NewPublicGitHubSource()
	if err != nil {
		fmt.Printf("Failed to create update source: %v\n", err)
		return
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
	if err != nil {
		fmt.Printf("Failed to create updater: %v\n", err)
		return
	}

	fmt.Printf("Current version: %s\nChecking for updates...\n", current)

	latest, found, err := updater.DetectLatest(context.Background(), selfupdate.ParseSlug("kesensoy/gci"))
	if err != nil {
		fmt.Printf("Update check failed: %v\n", err)
		return
	}
	if !found {
		fmt.Println("No release found for your OS/architecture.")
		return
	}

	if latest.LessOrEqual(current) {
		fmt.Println("Already up to date.")
		return
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		fmt.Printf("Could not locate executable: %v\n", err)
		return
	}

	if err := updater.UpdateTo(context.Background(), latest, exe); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Printf("Updated to %s\n", latest.Version())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
