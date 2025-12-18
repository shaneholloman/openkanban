# Architecture

## System Overview

Agent Board is a TUI application built with Go and Bubbletea that orchestrates AI coding agents across multiple isolated development environments.

```
┌─────────────────────────────────────────────────────────────────┐
│                         Agent Board TUI                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Backlog   │  │ In Progress │  │    Done     │              │
│  │  ┌───────┐  │  │  ┌───────┐  │  │  ┌───────┐  │              │
│  │  │Ticket │  │  │  │Ticket │  │  │  │Ticket │  │              │
│  │  └───────┘  │  │  └───────┘  │  │  └───────┘  │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────┘
         │                   │                   │
         ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Core Engine                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Ticket Store │  │ Git Manager  │  │Terminal Panes│          │
│  │  (JSON/SQL)  │  │  (worktrees) │  │   (PTY)      │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
         │                   │                   │
         ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────────────────┐
│                     System Layer                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Filesystem  │  │     Git      │  │  PTY/vt10x   │          │
│  │ (.openkanban) │  │  (worktrees) │  │ (terminals)  │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

## Component Breakdown

### 1. TUI Layer (`internal/ui/`)

Built with [Bubbletea](https://github.com/charmbracelet/bubbletea) (Elm architecture):

```go
// Model holds all application state
type Model struct {
    board      BoardModel      // Kanban columns and tickets
    focused    FocusState      // What's currently focused
    dialog     DialogModel     // Modal dialogs (create/edit/confirm)
    config     *Config         // Application configuration
    gitMgr     *git.Manager    // Git operations
    agentMgr   *agent.Manager  // Agent lifecycle
    store      store.Store     // Persistence
    width      int             // Terminal width
    height     int             // Terminal height
}

// Update handles all messages (Elm architecture)
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKeypress(msg)
    case tea.WindowSizeMsg:
        return m.handleResize(msg)
    case AgentStatusMsg:
        return m.handleAgentStatus(msg)
    // ...
    }
}
```

**Components:**

| Component | File | Purpose |
|-----------|------|---------|
| `App` | `app.go` | Root model, message routing |
| `Board` | `board.go` | Columns, ticket layout |
| `Ticket` | `ticket.go` | Single ticket card rendering |
| `Dialog` | `dialog.go` | Modal dialogs (create, edit, confirm) |
| `Help` | `help.go` | Help overlay |
| `Styles` | `styles.go` | Lipgloss styles (Catppuccin theme) |

### 2. Core Layer (`internal/core/`)

Business logic, decoupled from UI:

```go
// Ticket represents a single task
type Ticket struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Slug        string    `json:"slug"`
    Description string    `json:"description,omitempty"`
    Status      Status    `json:"status"`
    Agent       string    `json:"agent,omitempty"`
    Worktree    string    `json:"worktree,omitempty"`
    Branch      string    `json:"branch,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// Status represents ticket state
type Status string

const (
    StatusBacklog    Status = "backlog"
    StatusInProgress Status = "in_progress"
    StatusReview     Status = "review"
    StatusDone       Status = "done"
)

// Board manages collections of tickets
type Board struct {
    Columns []Column
    Tickets map[string]*Ticket
}
```

### 3. Git Layer (`internal/git/`)

Manages worktrees for isolated development:

```go
type Manager struct {
    repoRoot    string  // Root of main repository
    worktreeDir string  // Where worktrees live (.worktrees/)
}

// CreateWorktree creates an isolated worktree for a ticket
func (m *Manager) CreateWorktree(ticket *core.Ticket) error {
    // 1. Create branch: task/ticket-slug
    branch := fmt.Sprintf("task/%s", ticket.Slug)
    if err := m.createBranch(branch); err != nil {
        return err
    }
    
    // 2. Create worktree directory
    worktreePath := filepath.Join(m.worktreeDir, ticket.Slug)
    cmd := exec.Command("git", "worktree", "add", worktreePath, branch)
    return cmd.Run()
}

// RemoveWorktree cleans up a ticket's worktree
func (m *Manager) RemoveWorktree(ticket *core.Ticket) error {
    cmd := exec.Command("git", "worktree", "remove", ticket.Worktree)
    return cmd.Run()
}
```

### 4. Terminal Layer (`internal/terminal/`)

Embedded PTY-based terminal panes using `creack/pty` and `hinshun/vt10x`:

```go
// Pane represents an embedded terminal running an agent
type Pane struct {
	id      string
	pty     *os.File
	vt      *vt10x.VT
	cmd     *exec.Cmd
	workdir string
	width   int
	height  int
}

// Start launches a command in the PTY
func (p *Pane) Start(command string, args ...string) tea.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Dir = p.workdir
	
	pty, err := pty.Start(cmd)
	if err != nil {
		return nil
	}
	p.pty = pty
	p.cmd = cmd
	
	// Start reading output and updating vt10x terminal
	go p.readLoop()
	return p.tick()
}

// HandleKey forwards keyboard input to the PTY
func (p *Pane) HandleKey(msg tea.KeyMsg) tea.Msg {
	// Convert to escape sequence and write to PTY
	p.pty.Write(keyToBytes(msg))
	return nil
}

// View renders the terminal content from vt10x
func (p *Pane) View() string {
	return renderVT(p.vt, p.width, p.height)
}
```

### 5. Agent Layer (`internal/agent/`)

Agent configuration and status polling (lifecycle managed by Terminal layer):

```go
type Manager struct {
	config *config.Config
}

// GetAgentConfig returns config for an agent type
func (m *Manager) GetAgentConfig(agentType string) (*config.AgentConfig, bool) {
	cfg, ok := m.config.Agents[agentType]
	return &cfg, ok
}

// StatusPollInterval returns the configured polling interval
func (m *Manager) StatusPollInterval() time.Duration {
	return time.Duration(m.config.UI.RefreshInterval) * time.Second
}
```

### 5. Store Layer (`internal/store/`)

Persistence abstraction:

```go
type Store interface {
    Load() (*core.Board, error)
    Save(board *core.Board) error
    Close() error
}

// JSONStore implements Store with JSON file persistence
type JSONStore struct {
    path string
}

func (s *JSONStore) Load() (*core.Board, error) {
    data, err := os.ReadFile(s.path)
    if os.IsNotExist(err) {
        return core.NewBoard(), nil
    }
    var board core.Board
    return &board, json.Unmarshal(data, &board)
}

// SQLiteStore implements Store with SQLite (optional, for larger boards)
type SQLiteStore struct {
    db *sql.DB
}
```

## Data Flow

### Creating a Ticket

```
User presses 'n'
       │
       ▼
┌──────────────┐
│ Show Dialog  │ ← Dialog component renders
└──────────────┘
       │
       ▼ (user enters title, presses enter)
┌──────────────┐
│ Create Ticket│ ← core.NewTicket(title)
└──────────────┘
       │
       ▼
┌──────────────┐
│ Save to Store│ ← store.Save(board)
└──────────────┘
       │
       ▼
┌──────────────┐
│ Update Board │ ← Board re-renders with new ticket
└──────────────┘
```

### Moving Ticket to "In Progress"

```
User presses 'l' on backlog ticket
       │
       ▼
┌──────────────────┐
│ Update Status    │ ← ticket.Status = StatusInProgress
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Create Worktree  │ ← git worktree add .worktrees/slug task/slug
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Create PTY Pane  │ ← terminal.New() with PTY
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Start Agent      │ ← pane.Start("opencode") in worktree
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Save State       │ ← store.Save(board)
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Update UI        │ ← Ticket moves to "In Progress" column
└──────────────────┘
```

### Opening a Ticket

```
User presses 'enter' on ticket
       │
       ▼
┌──────────────────┐
│ Focus Terminal   │ ← Switch to ModeAgentView
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Show PTY Pane    │ ← Embedded terminal fills screen
└──────────────────┘
       │
       ▼ (user presses Ctrl-O to exit focus)
┌──────────────────┐
│ Return to Board  │ ← Switch back to ModeNormal
└──────────────────┘
```

## Event System

Bubbletea uses messages for all state changes:

```go
// Custom messages
type TicketCreatedMsg struct{ Ticket *core.Ticket }
type TicketMovedMsg struct{ Ticket *core.Ticket; From, To core.Status }
type TicketDeletedMsg struct{ ID string }
type AgentStatusMsg struct{ TicketID string; Status AgentStatus }
type WorktreeCreatedMsg struct{ TicketID string; Path string }
type ErrorMsg struct{ Err error }

// Commands that produce messages
func createTicketCmd(title string) tea.Cmd {
    return func() tea.Msg {
        ticket := core.NewTicket(title)
        return TicketCreatedMsg{Ticket: ticket}
    }
}

func pollAgentStatusCmd(ticketID string) tea.Cmd {
    return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
        status := agentMgr.GetStatus(ticketID)
        return AgentStatusMsg{TicketID: ticketID, Status: status}
    })
}
```

## Configuration

```yaml
# ~/.config/openkanban/config.yaml

# UI settings
theme: catppuccin-mocha  # or: catppuccin-latte, nord, dracula

# Columns
columns:
  - name: Backlog
    key: backlog
    color: "#89b4fa"  # blue
  - name: In Progress
    key: in_progress
    color: "#f9e2af"  # yellow
    spawn_agent: true  # Auto-spawn when ticket enters
  - name: Review
    key: review
    color: "#cba6f7"  # mauve
  - name: Done
    key: done
    color: "#a6e3a1"  # green
    cleanup_worktree: false  # Keep worktree on completion

# Git settings
git:
  worktree_dir: .worktrees
  branch_prefix: task/
  auto_push: false

# Agent settings
default_agent: opencode

agents:
  opencode:
    command: opencode
    args: []
    status_file: .opencode/status.json
  claude:
    command: claude
    args: ["--dangerously-skip-permissions"]
    status_file: .claude/status.json
  aider:
    command: aider
    args: ["--yes"]

# UI settings
ui:
  theme: catppuccin-mocha
  refresh_interval: 5
```

## Error Handling

Errors are surfaced through the message system:

```go
func (m Model) handleError(err error) (tea.Model, tea.Cmd) {
    // Log error
    log.Printf("Error: %v", err)
    
    // Show in status bar
    m.statusMessage = fmt.Sprintf("Error: %v", err)
    m.statusLevel = StatusError
    
    // Clear after 5 seconds
    return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
        return ClearStatusMsg{}
    })
}
```

## Testing Strategy

```
internal/
├── core/
│   ├── ticket.go
│   └── ticket_test.go      # Unit tests for ticket logic
├── git/
│   ├── worktree.go
│   └── worktree_test.go    # Integration tests (needs git)
├── agent/
│   ├── manager.go
│   └── manager_test.go     # Integration tests (needs tmux)
└── ui/
    ├── board.go
    └── board_test.go       # Snapshot tests for rendering
```

For UI testing, use Bubbletea's testing utilities:

```go
func TestBoardRender(t *testing.T) {
    board := NewBoard()
    board.AddTicket(core.NewTicket("Test ticket"))
    
    // Render and compare
    got := board.View()
    golden := loadGolden(t, "board_with_ticket.txt")
    if got != golden {
        t.Errorf("render mismatch:\n%s", diff(got, golden))
    }
}
```

## Future Considerations

### Plugin System
Allow custom agents via config:

```yaml
agents:
  custom:
    command: /path/to/my-agent
    args: ["--mode", "interactive"]
    status_command: "pgrep -f my-agent"
```

### Remote Agents
SSH-based agent spawning for remote development:

```yaml
remotes:
  dev-server:
    host: dev.example.com
    user: developer
    worktree_dir: ~/worktrees
```

### GitHub/GitLab Sync
Bi-directional sync with issue trackers:

```yaml
integrations:
  github:
    enabled: true
    repo: owner/repo
    sync_labels: true
```
