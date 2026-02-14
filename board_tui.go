package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gci/internal/usercfg"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type kanbanColumnView struct {
	title          string
	statusCategory string
	issues         []JiraIssue // current, possibly filtered/grouped view
	allIssues      []JiraIssue // raw, unfiltered data from last fetch
	allByScope     map[scopeFilter][]JiraIssue
	cursor         int
	offset         int // top index of the visible window
}

type dataLoadedMsg struct {
	columns []kanbanColumnView
}

type errMsg struct{ err error }

// lazyBatchLoadedMsg contains background-fetched data for a specific scope across columns
type lazyBatchLoadedMsg struct {
	scope   scopeFilter
	byIndex map[int][]JiraIssue // column index -> issues
}

type boardModel struct {
	cfg             *Config
	columns         []kanbanColumnView
	selectedCol     int
	loading         bool
	err             error
	curScope        scopeFilter
	width           int
	height          int
	filtering       bool
	filterInput     textinput.Model
	filter          string
	showingHelp     bool
	styles          boardStyles
	launchSetup     bool // request to launch setup wizard after TUI exits
	helpOffset      int  // scroll offset within help overlay
	pendingWorktree string
	pendingIssue    JiraIssue
	pendingClaude   bool // whether to spawn Claude after TUI exits
}

// newBoardStyles returns hardcoded dark theme styles
func newBoardStyles() boardStyles {
	return boardStyles{
		header:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		boxStyle:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).BorderForeground(lipgloss.Color("240")),
		boxActive:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).BorderForeground(lipgloss.Color("10")),
		selected:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")),
		muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		help:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		helpOverlay: lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("255")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")).Padding(1, 2),
		helpTitle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")),
		helpKey:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
		error:       lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
}

type boardStyles struct {
	header      lipgloss.Style
	title       lipgloss.Style
	boxStyle    lipgloss.Style
	boxActive   lipgloss.Style
	selected    lipgloss.Style
	muted       lipgloss.Style
	help        lipgloss.Style
	helpOverlay lipgloss.Style
	helpTitle   lipgloss.Style
	helpKey     lipgloss.Style
	error       lipgloss.Style
}

func initialBoardModel(cfg *Config) boardModel {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 256

	// Initialize hardcoded dark theme styles
	styles := newBoardStyles()

	// Load UI preferences
	uiPrefs := usercfg.GetUIPrefs()

	// Determine initial scope
	var initialScope scopeFilter
	if uiPrefs.LastScope != "" {
		initialScope = scopeFromString(uiPrefs.LastScope)
	} else {
		initialScope = getDefaultScope()
	}

	// Determine initial selected column
	var initialCol int
	if uiPrefs.LastSelectedCol >= 0 && uiPrefs.LastSelectedCol < 3 {
		initialCol = uiPrefs.LastSelectedCol
	}

	return boardModel{
		cfg: cfg,
		columns: []kanbanColumnView{
			{title: "To Do", statusCategory: "To Do"},
			{title: "In Progress", statusCategory: "In Progress"},
			{title: "Done", statusCategory: "Done"},
		},
		selectedCol: initialCol,
		loading:     true,
		curScope:    initialScope,
		filterInput: ti,
		styles:      styles,
	}
}

func (m boardModel) Init() tea.Cmd { return m.loadDataCmd() }

func (m boardModel) loadDataCmd() tea.Cmd {
	cfg := *m.cfg
	columns := make([]kanbanColumnView, len(m.columns))
	copy(columns, m.columns)
	filter := m.filter
	scope := m.curScope

	return func() tea.Msg {
		// Use concurrent fetching for standard scope-based mode
		return m.loadColumnsConcurrently(cfg, columns, scope, filter)
	}
}

// loadColumnsConcurrently fetches column data concurrently with proper worker limits and context
func (m boardModel) loadColumnsConcurrently(cfg Config, columns []kanbanColumnView, scope scopeFilter, filter string) tea.Msg {
	// Create context with timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use worker pool to limit concurrent requests
	const maxWorkers = 3
	semaphore := make(chan struct{}, maxWorkers)
	
	type columnResult struct {
		index  int
		issues []JiraIssue
		err    error
	}
	
	results := make(chan columnResult, len(columns))
	
	// Start workers for each column
	for i := range columns {
		go func(idx int, col kanbanColumnView) {
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- columnResult{index: idx, err: ctx.Err()}
				return
			}
			
			// Fetch issues with context
			issues, err := fetchColumnIssuesWithContext(ctx, &cfg, col.statusCategory, scope, 100)
			results <- columnResult{
				index:  idx,
				issues: issues,
				err:    err,
			}
		}(i, columns[i])
	}
	
	// Collect results with timeout
collectLoop:
	for completed := 0; completed < len(columns); completed++ {
		select {
		case result := <-results:
			if result.err != nil {
				if result.err == context.DeadlineExceeded || result.err == context.Canceled {
					// Context timeout or cancellation - return partial results
					break collectLoop
				}
				return errMsg{result.err}
			}
			
			idx := result.index
			issues := result.issues
			
			columns[idx].allIssues = issues
			if columns[idx].allByScope == nil {
				columns[idx].allByScope = make(map[scopeFilter][]JiraIssue)
			}
			columns[idx].allByScope[scope] = issues
			columns[idx].issues = m.filterAndGroupColumn(columns[idx].title, issues, filter)

			if columns[idx].cursor >= len(issues) {
				if len(issues) == 0 {
					columns[idx].cursor = 0
				} else {
					columns[idx].cursor = len(issues) - 1
				}
			}
			
		case <-ctx.Done():
			// Timeout - return partial results
			break collectLoop
		}
	}
	
	return dataLoadedMsg{columns: columns}
}

// loadScopeConcurrently loads a specific scope across all columns concurrently for background caching
func (m boardModel) loadScopeConcurrently(cfg Config, columns []kanbanColumnView, scope scopeFilter) lazyBatchLoadedMsg {
	// Create context with timeout for all operations  
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Use worker pool to limit concurrent requests
	const maxWorkers = 3
	semaphore := make(chan struct{}, maxWorkers)
	
	type scopeResult struct {
		index  int
		issues []JiraIssue
		err    error
	}
	
	results := make(chan scopeResult, len(columns))
	
	// Start workers for each column
	for i := range columns {
		go func(idx int, col kanbanColumnView) {
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- scopeResult{index: idx, err: ctx.Err()}
				return
			}
			
			// Fetch issues with context
			issues, err := fetchColumnIssuesWithContext(ctx, &cfg, col.statusCategory, scope, 100)
			results <- scopeResult{
				index:  idx,
				issues: issues,
				err:    err,
			}
		}(i, columns[i])
	}
	
	// Collect results with timeout
	byIdx := make(map[int][]JiraIssue, len(columns))
	
collectScopeLoop:	
	for completed := 0; completed < len(columns); completed++ {
		select {
		case result := <-results:
			if result.err != nil {
				// Ignore errors for background loading - just skip this column
				continue
			}
			
			byIdx[result.index] = result.issues
			
		case <-ctx.Done():
			// Timeout - return partial results
			break collectScopeLoop
		}
	}
	
	return lazyBatchLoadedMsg{scope: scope, byIndex: byIdx}
}

// filterAndGroupColumn applies a fuzzy text filter and then
// groups/partitions issues for display.
func (m boardModel) filterAndGroupColumn(title string, all []JiraIssue, filter string) []JiraIssue {
	if filter == "" {
		return reorderAndGroupIssues(title, all)
	}

	normalizedFilter := usercfg.NormalizeSearchText(filter)

	type scoredIssue struct {
		issue JiraIssue
		score int
	}
	var scored []scoredIssue
	for _, it := range all {
		keyScore := usercfg.FuzzyScore(normalizedFilter, usercfg.NormalizeSearchText(it.Key))
		summaryScore := usercfg.FuzzyScore(normalizedFilter, usercfg.NormalizeSearchText(it.Fields.Summary))
		bestScore := keyScore
		if summaryScore > bestScore {
			bestScore = summaryScore
		}
		if bestScore > 0 {
			scored = append(scored, scoredIssue{issue: it, score: bestScore})
		}
	}
	// Sort by score (highest first)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	result := make([]JiraIssue, len(scored))
	for i, s := range scored {
		result[i] = s.issue
	}
	return reorderAndGroupIssues(title, result)
}

// reorderAndGroupIssues returns a new slice where parent issues appear before their subtasks,
// and for To Do columns with mixed backlog/active statuses: non-backlog items (incl. promoted backlog parents of To Do subtasks)
// come before backlog items. Order is otherwise stable.
func reorderAndGroupIssues(columnTitle string, issues []JiraIssue) []JiraIssue {
	if len(issues) == 0 {
		return issues
	}
	// Build lookup maps and original order
	byKey := make(map[string]JiraIssue, len(issues))
	present := make(map[string]struct{}, len(issues))
	for _, it := range issues {
		byKey[it.Key] = it
		present[it.Key] = struct{}{}
	}

	isBacklog := func(it JiraIssue) bool {
		return strings.Contains(strings.ToLower(it.Fields.Status.Name), "backlog")
	}
	// Partition top vs backlog for To Do column
	topSet := make(map[string]struct{}, len(issues))
	backlogSet := make(map[string]struct{}, len(issues))
	if columnTitle == "To Do" {
		for _, it := range issues {
			if isBacklog(it) {
				backlogSet[it.Key] = struct{}{}
			} else {
				topSet[it.Key] = struct{}{}
			}
		}
		// Promote backlog parents that have at least one child in the topSet
		for _, it := range issues {
			if _, ok := topSet[it.Key]; !ok {
				continue
			}
			if it.Fields.IssueType.Subtask && it.Fields.Parent.Key != "" {
				if _, exists := present[it.Fields.Parent.Key]; exists {
					delete(backlogSet, it.Fields.Parent.Key)
					topSet[it.Fields.Parent.Key] = struct{}{}
				}
			}
		}
	} else {
		// Not the To Do column: keep everything in topSet to preserve original order
		for _, it := range issues {
			topSet[it.Key] = struct{}{}
		}
	}

	// Helper to append a parent and its children (only those present in a given allow-set)
	appendGroup := func(dst *[]JiraIssue, parent JiraIssue, allow map[string]struct{}) {
		// Parent first
		*dst = append(*dst, parent)
		// Then its children in original order
		for _, it := range issues {
			if it.Fields.IssueType.Subtask && it.Fields.Parent.Key == parent.Key {
				if _, ok := allow[it.Key]; ok {
					*dst = append(*dst, it)
				}
			}
		}
	}

	seen := make(map[string]struct{}, len(issues))
	out := make([]JiraIssue, 0, len(issues))

	// First pass: top group
	for _, it := range issues {
		if _, ok := topSet[it.Key]; !ok {
			continue
		}
		if _, done := seen[it.Key]; done {
			continue
		}
		// If subtask and parent present in set, skip now; it will be added under parent
		if it.Fields.IssueType.Subtask && it.Fields.Parent.Key != "" {
			if _, parentPresent := topSet[it.Fields.Parent.Key]; parentPresent {
				continue
			}
		}
		// Append this issue and its children
		appendGroup(&out, it, topSet)
		seen[it.Key] = struct{}{}
		for _, ch := range issues {
			if ch.Fields.IssueType.Subtask && ch.Fields.Parent.Key == it.Key {
				if _, ok := topSet[ch.Key]; ok {
					seen[ch.Key] = struct{}{}
				}
			}
		}
	}

	// Second pass: backlog group (for To Do only)
	if columnTitle == "To Do" {
		for _, it := range issues {
			if _, ok := backlogSet[it.Key]; !ok {
				continue
			}
			if _, done := seen[it.Key]; done {
				continue
			}
			if it.Fields.IssueType.Subtask && it.Fields.Parent.Key != "" {
				if _, parentPresent := backlogSet[it.Fields.Parent.Key]; parentPresent {
					continue
				}
			}
			appendGroup(&out, it, backlogSet)
			seen[it.Key] = struct{}{}
			for _, ch := range issues {
				if ch.Fields.IssueType.Subtask && ch.Fields.Parent.Key == it.Key {
					if _, ok := backlogSet[ch.Key]; ok {
						seen[ch.Key] = struct{}{}
					}
				}
			}
		}
	}

	return out
}

func (m boardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Keep cursor visible in each column after resize
		for i := range m.columns {
			m.ensureCursorVisible(&m.columns[i])
		}
		return m, nil
	case tea.KeyMsg:
		if m.showingHelp {
			key := msg.String()
			// Compute wrapped help lines and viewport
			lines, _, viewport := m.helpLayout()
			maxOffset := 0
			if viewport < len(lines) {
				maxOffset = len(lines) - viewport
			}
			switch key {
			case "q", "?", "esc":
				m.showingHelp = false
				return m, nil
			case "up", "k":
				if m.helpOffset > 0 {
					m.helpOffset--
				}
				return m, nil
			case "down", "j":
				if m.helpOffset < maxOffset {
					m.helpOffset++
				}
				return m, nil
			case "pgup":
				step := max(1, viewport-1)
				m.helpOffset = max(0, m.helpOffset-step)
				return m, nil
			case "pgdown":
				step := max(1, viewport-1)
				m.helpOffset = min(maxOffset, m.helpOffset+step)
				return m, nil
			case "home":
				m.helpOffset = 0
				return m, nil
			case "end":
				m.helpOffset = maxOffset
				return m, nil
			default:
				return m, nil
			}
		}
		if m.filtering {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyCtrlC:
				m.filtering = false
				return m, nil
			case tea.KeyEnter:
				// Exit filtering, fall through to normal key handling
				m.filtering = false
			default:
				// Live update filter as user types; no refetch
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.filter = m.filterInput.Value()
				// Re-derive filtered/grouped views locally
				for i := range m.columns {
					m.columns[i].issues = m.filterAndGroupColumn(m.columns[i].title, m.columns[i].allIssues, m.filter)
					m.ensureCursorVisible(&m.columns[i])
				}
				return m, cmd
			}
		}
		key := msg.String()
		switch {
		// Critical actions first to avoid conflicts with navigation keys
		case key == "q" || key == "ctrl+c":
			m.saveUIPreferences()
			return m, tea.Quit
		case key == "?":
			m.showingHelp = !m.showingHelp
			return m, nil
		case key == "w":
			// Mark to launch setup wizard after exiting TUI
			m.launchSetup = true
			m.saveUIPreferences()
			return m, tea.Quit
		case key == "s":
			// cycle through 4 scopes; switch instantly if cached, else show per-column loading and fetch in background
			m.curScope = (m.curScope + 1) % 4
			var missing []int
			for i := range m.columns {
				if data, ok := m.columns[i].allByScope[m.curScope]; ok {
					m.columns[i].allIssues = data
					m.columns[i].issues = m.filterAndGroupColumn(m.columns[i].title, data, m.filter)
				} else {
					missing = append(missing, i)
				}
				m.ensureCursorVisible(&m.columns[i])
			}
			if len(missing) == 0 {
				return m, nil
			}
			sc := m.curScope
			cfg := *m.cfg
			colsSnapshot := make([]kanbanColumnView, len(m.columns))
			copy(colsSnapshot, m.columns)
			// mark columns as loading
			for _, i := range missing {
				// show a temporary empty list with a loading indicator in View
				m.columns[i].issues = nil
			}
			return m, func() tea.Msg {
				byIdx := make(map[int][]JiraIssue, len(colsSnapshot))
				for i := range colsSnapshot {
					issues, err := fetchColumnIssues(&cfg, colsSnapshot[i].statusCategory, sc, 100)
					if err != nil {
						continue
					}
					byIdx[i] = issues
				}
				return lazyBatchLoadedMsg{scope: sc, byIndex: byIdx}
			}
		case key == "/":
			m.filtering = true
			m.filterInput.SetValue(m.filter)
			m.filterInput.Focus()
			return m, nil
		case key == "o":
			if issue, ok := m.currentIssue(); ok {
				_ = openIssueInBrowser(m.cfg, issue)
			}
		case key == "b":
			// If filtered results are in a different column, jump there
			if _, ok := m.currentIssue(); !ok {
				for i := range m.columns {
					if len(m.columns[i].issues) > 0 {
						m.selectedCol = i
						m.columns[i].cursor = 0
						break
					}
				}
			}
			if issue, ok := m.currentIssue(); ok {
				branch := createBranchName(issue)
				if err := createOrCheckoutBranch(branch); err != nil {
					m.err = err
					return m, nil
				}
				m.saveUIPreferences()
				return m, tea.Quit
			}
		case key == "enter":
			// Interactive Mode: behavior depends on EnableClaude and EnableWorktrees config
			if _, ok := m.currentIssue(); !ok {
				for i := range m.columns {
					if len(m.columns[i].issues) > 0 {
						m.selectedCol = i
						m.columns[i].cursor = 0
						break
					}
				}
			}
			if issue, ok := m.currentIssue(); ok {
				branch := createBranchName(issue)
				m.pendingIssue = issue

				if m.cfg.EnableWorktrees {
					// Worktree path
					result := createOrCheckoutWorktree(branch)
					if result.Error != nil {
						// Fallback to branch in current directory
						if err := createOrCheckoutBranch(branch); err != nil {
							m.err = result.Error
							return m, nil
						}
						m.saveUIPreferences()
						fmt.Printf("\n\033[92mBranch ready: %s\033[0m\n", branch)
						m.pendingWorktree = "."
					} else {
						m.saveUIPreferences()
						fmt.Printf("\n\033[92mWorktree ready: %s\033[0m\n", result.Path)
						m.pendingWorktree = result.Path
					}
				} else {
					// Branch-only path
					if err := createOrCheckoutBranch(branch); err != nil {
						m.err = err
						return m, nil
					}
					m.saveUIPreferences()
					fmt.Printf("\n\033[92mBranch ready: %s\033[0m\n", branch)
					m.pendingWorktree = "."
				}

				if m.cfg.EnableClaude {
					fmt.Printf("\033[93mSpawning Claude with ticket context...\033[0m\n")
					m.pendingClaude = true
				} else {
					// Print ticket info for non-Claude flow
					description := extractDescriptionText(issue)
					fmt.Printf("\n\033[96m%s: %s\033[0m\n", issue.Key, issue.Fields.Summary)
					if description != "" {
						fmt.Printf("\n%s\n", description)
					}
					fmt.Println()
				}
				return m, tea.Quit
			}
		case key == "r":
			m.loading = true
			return m, m.loadDataCmd()
		// Navigation last so action keys like w/s don't get shadowed if users add them to movement
		case key == "l" || key == "right" || key == "tab":
			m.selectedCol = (m.selectedCol + 1) % len(m.columns)
			if len(m.columns) > 0 {
				m.ensureCursorVisible(&m.columns[m.selectedCol])
			}
		case key == "h" || key == "left" || key == "shift+tab":
			m.selectedCol = (m.selectedCol - 1 + len(m.columns)) % len(m.columns)
			if len(m.columns) > 0 {
				m.ensureCursorVisible(&m.columns[m.selectedCol])
			}
		case key == "j" || key == "down":
			col := &m.columns[m.selectedCol]
			if len(col.issues) > 0 && col.cursor < len(col.issues)-1 {
				col.cursor++
				m.ensureCursorVisible(col)
			}
		case key == "k" || key == "up":
			col := &m.columns[m.selectedCol]
			if len(col.issues) > 0 && col.cursor > 0 {
				col.cursor--
				m.ensureCursorVisible(col)
			}
		}
		return m, nil
	case dataLoadedMsg:
		m.loading = false
		m.err = nil
		m.columns = msg.columns
		for i := range m.columns {
			m.ensureCursorVisible(&m.columns[i])
		}
		// Prefetch other scopes immediately (in parallel) to guarantee instant scope switches
		scopes := []scopeFilter{scopeMineOrReported, scopeMine, scopeReported, scopeUnassigned}
		colsSnapshot := make([]kanbanColumnView, len(m.columns))
		copy(colsSnapshot, m.columns)
		cfg := *m.cfg
		cmds := make([]tea.Cmd, 0, len(scopes)-1)
		for _, sc := range scopes {
			if sc == m.curScope {
				continue
			}
			scLocal := sc // This alone isn't enough - need to pass to closure
			cmds = append(cmds, func(scope scopeFilter) tea.Cmd {
				return func() tea.Msg {
					return m.loadScopeConcurrently(cfg, colsSnapshot, scope)
				}
			}(scLocal))
		}
		return m, tea.Batch(cmds...)
	case lazyBatchLoadedMsg:
		// Populate caches and, if current scope matches, refresh visible data
		for idx, issues := range msg.byIndex {
			if idx < 0 || idx >= len(m.columns) {
				continue
			}
			if m.columns[idx].allByScope == nil {
				m.columns[idx].allByScope = make(map[scopeFilter][]JiraIssue)
			}
			m.columns[idx].allByScope[msg.scope] = issues
			if msg.scope == m.curScope {
				m.columns[idx].allIssues = issues
				m.columns[idx].issues = m.filterAndGroupColumn(m.columns[idx].title, issues, m.filter)
				m.ensureCursorVisible(&m.columns[idx])
			}
		}
		return m, nil
	case errMsg:
		m.loading = false
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

func (m boardModel) View() string {
	// Show current mode (scope)
	modeStr := fmt.Sprintf("Scope: %s", scopeToString(m.curScope))

	header := m.styles.header.Render(clip(fmt.Sprintf("Personal Kanban — Projects: %s — %s", strings.Join(m.cfg.Projects, ","), modeStr), m.width))
	// Compact help to avoid overflowing small terminals; full help with '?'
	help := m.styles.help.Render(clip("(? help • q quit • arrows/hjkl move • / filter • b branch • enter interactive)", m.width))

	cols := len(m.columns)
	if cols == 0 {
		return header + "\n" + "No columns configured" + "\n"
	}

	// Column width percentages: To Do 35%, In Progress 35%, Done 30%
	var colWidths []int
	if cols > 0 {
		// Leave some margin for borders/padding
		usableWidth := m.width - 6 // account for borders and spacing
		colWidths = []int{
			int(float64(usableWidth) * 0.35), // To Do: 35%
			int(float64(usableWidth) * 0.35), // In Progress: 35%
			int(float64(usableWidth) * 0.30), // Done: 30%
		}
		// Ensure minimum widths
		for i := range colWidths {
			colWidths[i] = max(16, colWidths[i])
		}
	}

	// Compute how many list rows are available per column for ITEMS (not including
	// the top/bottom indicator lines).
	itemsWindow := m.itemsWindowCount()

	rendered := make([]string, cols)
	for i, c := range m.columns {
		var items []string
		if len(c.issues) == 0 {
			// Show loading only if we have no cached data for the current scope.
			// If cached data exists but is empty, show (empty).
			if _, ok := c.allByScope[m.curScope]; !ok {
				items = []string{m.styles.muted.Render("(loading…)")}
			} else {
				items = []string{m.styles.muted.Render("(empty)")}
			}
		} else {
			start := m.columns[i].offset
			end := min(len(c.issues), start+itemsWindow)

			// Top indicator or spacer
			if start > 0 {
				items = append(items, m.styles.muted.Render(fmt.Sprintf("… %d above", start)))
			} else {
				items = append(items, "")
			}
			// Pre-scan: show section tags only when To Do has a mix of backlog + non-backlog
			hasBacklogMix := false
			if c.title == "To Do" {
				hasBacklog, hasNonBacklog := false, false
				for _, it := range c.issues {
					if strings.Contains(strings.ToLower(it.Fields.Status.Name), "backlog") {
						hasBacklog = true
					} else {
						hasNonBacklog = true
					}
					if hasBacklog && hasNonBacklog {
						hasBacklogMix = true
						break
					}
				}
			}
			for idx := start; idx < end; idx++ {
				// Indent subtasks under parent
				indent := ""
				it := c.issues[idx]
				if it.Fields.IssueType.Subtask && it.Fields.Parent.Key != "" {
					indent = "  └─ "
				}
				// Inline tags when To Do column has mixed backlog and active statuses
				sectionTag := ""
				if hasBacklogMix {
					if strings.Contains(strings.ToLower(it.Fields.Status.Name), "backlog") {
						sectionTag = "[Backlog] "
					} else {
						sectionTag = "[To Do] "
					}
				}
				// Build basic line
				basicLine := fmt.Sprintf("%s — %s", it.Key, it.Fields.Summary)

				// Add extra fields if enabled
				uiPrefs := usercfg.GetUIPrefs()
				var extraTags []string
				if uiPrefs.ShowExtraFields {
					// Add assignee tag
					if it.Fields.Assignee.DisplayName != "" {
						// Use first name only to save space
						assigneeParts := strings.Fields(it.Fields.Assignee.DisplayName)
						if len(assigneeParts) > 0 {
							assigneeName := assigneeParts[0]
							if len(assigneeName) > 8 {
								assigneeName = assigneeName[:8]
							}
							extraTags = append(extraTags, fmt.Sprintf("@%s", assigneeName))
						} else {
							extraTags = append(extraTags, "@unknown")
						}
					} else {
						extraTags = append(extraTags, "@unassigned")
					}

					// Add priority tag
					if it.Fields.Priority.Name != "" {
						priority := it.Fields.Priority.Name
						// Abbreviate common priority names
						switch strings.ToLower(priority) {
						case "critical":
							priority = "CRIT"
						case "high":
							priority = "HIGH"
						case "medium":
							priority = "MED"
						case "low":
							priority = "LOW"
						case "lowest":
							priority = "MIN"
						}
						if len(priority) > 4 {
							priority = priority[:4]
						}
						extraTags = append(extraTags, fmt.Sprintf("P:%s", priority))
					}
				}

				// Combine line with tags
				var line string
				if len(extraTags) > 0 {
					tagStr := "[" + strings.Join(extraTags, " ") + "]"
					line = indent + sectionTag + basicLine + " " + tagStr
				} else {
					line = indent + sectionTag + basicLine
				}
				if i == m.selectedCol && idx == m.columns[i].cursor {
					items = append(items, m.styles.selected.Render(clip(line, colWidths[i]-4)))
				} else {
					items = append(items, clip(line, colWidths[i]-4))
				}
			}
			// Bottom indicator or spacer
			if end < len(c.issues) {
				remaining := len(c.issues) - end
				items = append(items, m.styles.muted.Render(fmt.Sprintf("… %d below", remaining)))
			} else {
				items = append(items, "")
			}
		}
		box := m.styles.boxStyle
		if i == m.selectedCol {
			box = m.styles.boxActive
		}
		title := m.styles.title.Render(c.title)
		rendered[i] = box.Width(colWidths[i]).Render(title + "\n" + strings.Join(items, "\n"))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	if m.filtering {
		return header + "\n" + help + "\n\n" + board + "\n\nFilter: " + m.filterInput.View()
	}
	footer := ""
	if m.err != nil {
		footer = "\n" + m.styles.error.Render("Error: "+m.err.Error())
	} else if m.loading {
		footer = "\n" + m.styles.muted.Render("Loading...")
	}
	if m.filter != "" {
		footer += "\n" + m.styles.muted.Render("Filter: "+m.filter)
	}
	baseView := header + "\n" + help + "\n\n" + board + footer + "\n"

	if m.showingHelp {
		return m.renderWithHelpOverlay(baseView)
	}

	return baseView
}

func (m boardModel) renderWithHelpOverlay(baseView string) string {
	lines, overlayWidth, viewport := m.helpLayout()
	// Clamp offset
	maxOffset := 0
	if viewport < len(lines) {
		maxOffset = len(lines) - viewport
	}
	if m.helpOffset > maxOffset {
		m.helpOffset = maxOffset
	}
	if m.helpOffset < 0 {
		m.helpOffset = 0
	}
	// Slice visible content
	start := m.helpOffset
	end := min(len(lines), start+viewport)
	visible := lines[start:end]
	helpContent := strings.Join(visible, "\n")
	overlayHeight := viewport + 3

	// Position overlay in center
	y := max(0, (m.height-overlayHeight)/2)

	// Create the overlay
	// Footer with position and controls
	pos := fmt.Sprintf("%d/%d lines — ↑/↓ PgUp/PgDn Home/End — q/? close", end, len(lines))
	helpBlock := helpContent + "\n" + m.styles.muted.Render(pos)
	overlay := m.styles.helpOverlay.Width(overlayWidth).Render(helpBlock)

	// For now, just overlay it on top of the base view
	// This is a simple approach - could be enhanced with proper layering
	baseLines := strings.Split(baseView, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Ensure we have enough base lines
	for len(baseLines) < y+len(overlayLines) {
		baseLines = append(baseLines, "")
	}

	// Overlay the help content
	for i, overlayLine := range overlayLines {
		if y+i < len(baseLines) {
			// Simple overlay - replace the line
			baseLines[y+i] = overlayLine
		}
	}

	return strings.Join(baseLines, "\n")
}

// helpLayout computes wrapped help lines, target overlay width, and viewport height (content rows)
func (m boardModel) helpLayout() ([]string, int, int) {
	helpContent := m.buildHelpContent()
	// Width bounds
	overlayWidth := min(80, max(40, m.width-8))
	// Wrap
	contentLines := strings.Split(helpContent, "\n")
	wrapped := make([]string, 0, len(contentLines))
	wrapWidth := max(10, overlayWidth-4)
	for _, line := range contentLines {
		for len(line) > wrapWidth {
			wrapped = append(wrapped, line[:wrapWidth])
			line = line[wrapWidth:]
		}
		wrapped = append(wrapped, line)
	}
	// Viewport rows for content (exclude padding/footer lines)
	viewport := max(3, min(m.height-4, len(wrapped)+3)-3)
	return wrapped, overlayWidth, viewport
}

func (m boardModel) buildHelpContent() string {
	title := m.styles.helpTitle.Render("🔧 Personal Kanban - Keyboard Shortcuts")

	helpLines := []string{
		m.styles.helpKey.Render("q/ctrl+c") + "    Quit application",
		m.styles.helpKey.Render("?") + "           Toggle this help overlay",
		"",
		m.styles.helpTitle.Render("Navigation:"),
		m.styles.helpKey.Render("hjkl/arrows") + " Navigate",
		m.styles.helpKey.Render("tab/shift+tab") + " Switch column",
		"",
		m.styles.helpTitle.Render("Actions:"),
		m.styles.helpKey.Render("r") + "           Refresh all columns",
		m.styles.helpKey.Render("s") + "           Cycle scope (assigned/reported/unassigned)",
		m.styles.helpKey.Render("/") + "           Filter issues (live search)",
		m.styles.helpKey.Render("o") + "           Open selected issue in browser",
		m.styles.helpKey.Render("b") + "           Create/checkout branch for issue",
		m.styles.helpKey.Render("enter") + "       Interactive Mode",
		m.styles.helpKey.Render("w") + "           Open setup wizard",
		"",
		m.styles.helpTitle.Render("Tips:"),
		"  • Use filters to quickly find issues",
		"  • Scope cycling preloads data for instant switching",
		"  • Branch names are auto-generated from issue key + summary",
		"  • Configure Claude AI and worktrees via gci setup",
	}

	return title + "\n\n" + strings.Join(helpLines, "\n") + "\n\n" + m.styles.muted.Render("Press ? again to close")
}

func (m boardModel) currentIssue() (JiraIssue, bool) {
	if len(m.columns) == 0 {
		return JiraIssue{}, false
	}
	c := m.columns[m.selectedCol]
	if len(c.issues) == 0 {
		return JiraIssue{}, false
	}
	return c.issues[c.cursor], true
}

// viewportItemsHeight calculates how many rows of items can be displayed per column
// given the current terminal height and rough space usage of headers/footers.
func (m boardModel) viewportItemsHeight() int {
	reserved := 5
	if m.filtering {
		reserved += 2
	}
	avail := max(5, m.height-reserved)
	return max(1, avail-3)
}

// itemsWindowCount returns the number of item rows we draw, excluding the two
// indicator lines (top and bottom). This keeps ensureCursorVisible and View aligned.
func (m boardModel) itemsWindowCount() int {
	base := m.viewportItemsHeight()
	if base <= 2 {
		return 1
	}
	return base - 2
}

// ensureCursorVisible adjusts the column offset so that the cursor stays within the
// visible window, honoring the up/down indicators.
func (m boardModel) ensureCursorVisible(c *kanbanColumnView) {
	if len(c.issues) == 0 {
		c.offset = 0
		c.cursor = 0
		return
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor > len(c.issues)-1 {
		c.cursor = len(c.issues) - 1
	}
	vh := m.itemsWindowCount()
	if c.cursor < c.offset {
		c.offset = c.cursor
	}
	if c.cursor >= c.offset+vh {
		c.offset = c.cursor - vh + 1
	}
	maxOffset := 0
	if len(c.issues) > vh {
		maxOffset = len(c.issues) - vh
	}
	if c.offset > maxOffset {
		c.offset = maxOffset
	}
	if c.offset < 0 {
		c.offset = 0
	}
}

func scopeToString(s scopeFilter) string {
	switch s {
	case scopeMineOrReported:
		return "Assigned or Reported by Me"
	case scopeMine:
		return "Assigned to Me"
	case scopeReported:
		return "Reported by Me"
	case scopeUnassigned:
		return "Unassigned"
	}
	return ""
}

func scopeFromString(s string) scopeFilter {
	switch s {
	case "assigned_or_reported", "Assigned or Reported by Me":
		return scopeMineOrReported
	case "assigned", "Assigned to Me":
		return scopeMine
	case "reported", "Reported by Me":
		return scopeReported
	case "unassigned", "Unassigned":
		return scopeUnassigned
	default:
		return scopeMineOrReported
	}
}

func scopeToConfigString(s scopeFilter) string {
	switch s {
	case scopeMineOrReported:
		return "assigned_or_reported"
	case scopeMine:
		return "assigned"
	case scopeReported:
		return "reported"
	case scopeUnassigned:
		return "unassigned"
	}
	return "assigned_or_reported"
}

func (m boardModel) saveUIPreferences() {
	// Get current column widths if available
	var colWidths []int
	if m.width > 0 {
		usableWidth := m.width - 6
		colWidths = []int{
			int(float64(usableWidth) * 0.35), // To Do: 35%
			int(float64(usableWidth) * 0.35), // In Progress: 35%
			int(float64(usableWidth) * 0.30), // Done: 30%
		}
		// Ensure minimum widths
		for i := range colWidths {
			colWidths[i] = max(16, colWidths[i])
		}
	}

	prefs := usercfg.UIPreferences{
		LastScope:       scopeToConfigString(m.curScope),
		ColumnWidths:    colWidths,
		LastSelectedCol: m.selectedCol,
	}

	// Save preferences (ignore errors as this is best-effort)
	_ = usercfg.SaveUIPrefs(prefs)
}

func StartBoard(cfg *Config) error {
	model := initialBoardModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()

	// Save UI preferences when the program exits
	if bm, ok := finalModel.(boardModel); ok {
		bm.saveUIPreferences()
		if bm.launchSetup {
			// Launch setup wizard synchronously after TUI exits
			runSetup(nil, nil)
		}
		// Spawn Claude in worktree/branch dir if Interactive Mode requested it
		if bm.pendingClaude && bm.pendingWorktree != "" {
			if err := spawnClaudeWithContext(bm.pendingWorktree, bm.pendingIssue); err != nil {
				fmt.Fprintf(os.Stderr, "Error spawning Claude: %v\n", err)
				return err
			}
		}
	}

	return err
}

// clip is a local helper similar to truncate but safe for narrow widths
func getDefaultScope() scopeFilter {
	config := usercfg.GetRuntimeConfig()
	switch config.DefaultScope {
	case "assigned":
		return scopeMine
	case "reported":
		return scopeReported
	case "unassigned":
		return scopeUnassigned
	default:
		return scopeMineOrReported
	}
}

func clip(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	if w <= 3 {
		return s[:w]
	}
	return s[:w-3] + "..."
}
