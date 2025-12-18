package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/techdufus/openkanban/internal/agent"
	"github.com/techdufus/openkanban/internal/board"
	"github.com/techdufus/openkanban/internal/config"
	"github.com/techdufus/openkanban/internal/git"
	"github.com/techdufus/openkanban/internal/terminal"
)

// Mode represents the current UI mode
type Mode string

const (
	ModeNormal       Mode = "NORMAL"
	ModeInsert       Mode = "INSERT"
	ModeCommand      Mode = "COMMAND"
	ModeHelp         Mode = "HELP"
	ModeConfirm      Mode = "CONFIRM"
	ModeCreateTicket Mode = "CREATE"
	ModeAgentView    Mode = "AGENT"
	ModeSettings     Mode = "SETTINGS"
)

const (
	minColumnWidth = 20
	columnOverhead = 5 // border (2) + padding (2) + margin (1)
)

// Model is the main Bubbletea model
type Model struct {
	// Configuration
	config *config.Config

	// Data
	board    *board.Board
	boardDir string

	// Managers
	agentMgr    *agent.Manager
	worktreeMgr *git.WorktreeManager

	// UI state
	mode         Mode
	activeColumn int
	activeTicket int
	width        int
	height       int
	spinner      spinner.Model
	scrollOffset int

	// Cached column tickets
	columnTickets [][]*board.Ticket

	// Overlay state
	showHelp    bool
	showConfirm bool
	confirmMsg  string
	confirmFn   func() tea.Cmd

	// Create ticket form
	titleInput textinput.Model

	// Error/notification
	notification string
	notifyTime   time.Time

	// Terminal panes for embedded agent sessions
	panes       map[board.TicketID]*terminal.Pane
	focusedPane board.TicketID // "" = board view, otherwise pane is focused

	// Settings UI state
	settingsIndex   int             // which setting field is selected
	settingsEditing bool            // true when editing a field value
	settingsInput   textinput.Model // input for editing string values
}

func NewModel(cfg *config.Config, b *board.Board, boardDir string, agentMgr *agent.Manager, worktreeMgr *git.WorktreeManager) *Model {
	ti := textinput.New()
	ti.Placeholder = "Enter ticket title..."
	ti.CharLimit = 100
	ti.Width = 40

	si := textinput.New()
	si.CharLimit = 200
	si.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Meter
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))

	m := &Model{
		config:        cfg,
		board:         b,
		boardDir:      boardDir,
		agentMgr:      agentMgr,
		worktreeMgr:   worktreeMgr,
		mode:          ModeNormal,
		titleInput:    ti,
		settingsInput: si,
		spinner:       sp,
		panes:         make(map[board.TicketID]*terminal.Pane),
	}
	m.refreshColumnTickets()
	return m
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tickAgentStatus(m.agentMgr.StatusPollInterval()),
		m.spinner.Tick,
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.focusedPane != "" {
			if pane, ok := m.panes[m.focusedPane]; ok {
				pane.SetSize(m.width, m.height-2)
			}
		}
		return m, nil

	case terminal.OutputMsg, terminal.RenderTickMsg:
		return m.handleTerminalMsg(msg)

	case terminal.ExitMsg:
		delete(m.panes, board.TicketID(msg.PaneID))
		if m.focusedPane == board.TicketID(msg.PaneID) {
			m.mode = ModeNormal
			m.focusedPane = ""
			m.notify("Agent exited")
		}
		return m, nil

	case terminal.ExitFocusMsg:
		m.mode = ModeNormal
		m.focusedPane = ""
		return m, nil

	case agentStatusMsg:
		m.agentMgr.PollStatuses(m.board.Tickets)
		return m, tickAgentStatus(m.agentMgr.StatusPollInterval())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case notificationMsg:
		if time.Since(m.notifyTime) > 3*time.Second {
			m.notification = ""
		}
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c", "q":
		if m.mode == ModeNormal {
			return m, tea.Quit
		}
	case "esc":
		if m.mode == ModeAgentView {
			break
		}
		m.mode = ModeNormal
		m.showHelp = false
		m.showConfirm = false
		m.titleInput.Blur()
		return m, nil
	case "?":
		if m.mode != ModeAgentView {
			m.showHelp = !m.showHelp
			return m, nil
		}
	}

	// Mode-specific handling
	if m.showHelp {
		// Any key closes help
		m.showHelp = false
		return m, nil
	}

	if m.showConfirm {
		return m.handleConfirm(msg)
	}

	switch m.mode {
	case ModeNormal:
		return m.handleNormalMode(msg)
	case ModeCommand:
		return m.handleCommandMode(msg)
	case ModeCreateTicket:
		return m.handleCreateTicketMode(msg)
	case ModeAgentView:
		return m.handleAgentViewMode(msg)
	case ModeSettings:
		return m.handleSettingsMode(msg)
	}

	return m, nil
}

// handleNormalMode processes keys in normal mode
func (m *Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// Navigation
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		m.moveTicket(1)
	case "k", "up":
		m.moveTicket(-1)
	case "g":
		m.activeTicket = 0
	case "G":
		if len(m.columnTickets) > m.activeColumn {
			m.activeTicket = len(m.columnTickets[m.activeColumn]) - 1
			if m.activeTicket < 0 {
				m.activeTicket = 0
			}
		}

	// Actions
	case "n":
		return m.createNewTicket()
	case "enter":
		return m.attachToAgent()
	case "d":
		return m.confirmDeleteTicket()
	case " ":
		return m.quickMoveTicket()
	case "s":
		return m.spawnAgent()
	case "S":
		return m.stopAgent()

	// Command mode
	case ":":
		m.mode = ModeCommand

	// Settings
	case "O":
		m.mode = ModeSettings
		m.settingsIndex = 0
		m.settingsEditing = false
	}

	return m, nil
}

// handleCommandMode processes keys in command mode
func (m *Model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Execute command
		m.mode = ModeNormal
	case "esc":
		m.mode = ModeNormal
	}
	return m, nil
}

// handleConfirm processes keys in confirm dialog
func (m *Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.showConfirm = false
		if m.confirmFn != nil {
			return m, m.confirmFn()
		}
	case "n", "N", "esc":
		m.showConfirm = false
	}
	return m, nil
}

func (m *Model) handleAgentViewMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pane, ok := m.panes[m.focusedPane]
	if !ok {
		m.mode = ModeNormal
		m.focusedPane = ""
		return m, nil
	}

	switch msg.String() {
	case "tab":
		m.focusedPane = m.nextActivePane()
		return m, nil
	}

	if result := pane.HandleKey(msg); result != nil {
		if _, isExit := result.(terminal.ExitFocusMsg); isExit {
			m.mode = ModeNormal
			m.focusedPane = ""
		}
	}

	return m, nil
}

func (m *Model) nextActivePane() board.TicketID {
	var activePanes []board.TicketID
	for id, pane := range m.panes {
		if pane.Running() {
			activePanes = append(activePanes, id)
		}
	}
	if len(activePanes) == 0 {
		return ""
	}

	currentIdx := -1
	for i, id := range activePanes {
		if id == m.focusedPane {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + 1) % len(activePanes)
	return activePanes[nextIdx]
}

func (m *Model) handleCreateTicketMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.titleInput.Value())
		if title == "" {
			m.notify("Title cannot be empty")
			return m, nil
		}
		ticket := board.NewTicket(title)
		ticket.Status = m.board.Columns[m.activeColumn].Status
		m.board.AddTicket(ticket)
		m.refreshColumnTickets()
		m.saveBoard()
		m.mode = ModeNormal
		m.titleInput.Blur()
		m.notify("Created: " + title)
		return m, nil
	case "esc":
		m.mode = ModeNormal
		m.titleInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

type settingsField struct {
	key   string
	label string
	kind  string
}

var settingsFields = []settingsField{
	{"default_agent", "Default Agent", "string"},
	{"worktree_base", "Worktree Base", "string"},
	{"auto_spawn_agent", "Auto Spawn Agent", "bool"},
	{"auto_create_branch", "Auto Create Branch", "bool"},
	{"branch_prefix", "Branch Prefix", "string"},
	{"tmux_prefix", "Tmux Prefix", "string"},
}

func (m *Model) handleSettingsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsEditing {
		return m.handleSettingsEdit(msg)
	}

	switch msg.String() {
	case "j", "down":
		m.settingsIndex++
		if m.settingsIndex >= len(settingsFields) {
			m.settingsIndex = len(settingsFields) - 1
		}
	case "k", "up":
		m.settingsIndex--
		if m.settingsIndex < 0 {
			m.settingsIndex = 0
		}
	case "enter", " ":
		return m.enterSettingsEdit()
	case "esc", "q":
		m.mode = ModeNormal
	}
	return m, nil
}

func (m *Model) handleSettingsEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	field := settingsFields[m.settingsIndex]

	switch msg.String() {
	case "enter":
		m.applySettingsValue(field.key, m.settingsInput.Value())
		m.settingsEditing = false
		m.settingsInput.Blur()
		m.saveBoard()
		m.notify("Settings saved")
		return m, nil
	case "esc":
		m.settingsEditing = false
		m.settingsInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.settingsInput, cmd = m.settingsInput.Update(msg)
	return m, cmd
}

func (m *Model) enterSettingsEdit() (tea.Model, tea.Cmd) {
	field := settingsFields[m.settingsIndex]
	s := &m.board.Settings

	if field.kind == "bool" {
		switch field.key {
		case "auto_spawn_agent":
			s.AutoSpawnAgent = !s.AutoSpawnAgent
		case "auto_create_branch":
			s.AutoCreateBranch = !s.AutoCreateBranch
		}
		m.saveBoard()
		m.notify("Setting toggled")
		return m, nil
	}

	m.settingsEditing = true
	m.settingsInput.SetValue(m.getSettingsValue(field.key))
	m.settingsInput.Focus()
	return m, textinput.Blink
}

func (m *Model) getSettingsValue(key string) string {
	s := &m.board.Settings
	switch key {
	case "default_agent":
		return s.DefaultAgent
	case "worktree_base":
		return s.WorktreeBase
	case "branch_prefix":
		return s.BranchPrefix
	case "tmux_prefix":
		return s.TmuxPrefix
	}
	return ""
}

func (m *Model) applySettingsValue(key, value string) {
	s := &m.board.Settings
	switch key {
	case "default_agent":
		s.DefaultAgent = value
	case "worktree_base":
		s.WorktreeBase = value
	case "branch_prefix":
		s.BranchPrefix = value
	case "tmux_prefix":
		s.TmuxPrefix = value
	}
}

// Navigation helpers
func (m *Model) moveColumn(delta int) {
	m.activeColumn += delta
	if m.activeColumn < 0 {
		m.activeColumn = 0
	}
	if m.activeColumn >= len(m.board.Columns) {
		m.activeColumn = len(m.board.Columns) - 1
	}
	m.activeTicket = 0
	m.ensureColumnVisible()
}

func (m *Model) ensureColumnVisible() {
	colWidth := m.calcColumnWidth()
	visibleCols := m.visibleColumnCount(colWidth)

	if m.activeColumn < m.scrollOffset {
		m.scrollOffset = m.activeColumn
	} else if m.activeColumn >= m.scrollOffset+visibleCols {
		m.scrollOffset = m.activeColumn - visibleCols + 1
	}

	maxOffset := len(m.board.Columns) - visibleCols
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m *Model) calcColumnWidth() int {
	if m.width == 0 || len(m.board.Columns) == 0 {
		return minColumnWidth
	}

	numCols := len(m.board.Columns)
	totalOverhead := numCols * columnOverhead
	colWidth := (m.width - totalOverhead) / numCols

	if colWidth < minColumnWidth {
		colWidth = minColumnWidth
	}
	return colWidth
}

func (m *Model) visibleColumnCount(colWidth int) int {
	if m.width == 0 {
		return len(m.board.Columns)
	}
	visible := m.width / (colWidth + columnOverhead)
	if visible < 1 {
		visible = 1
	}
	if visible > len(m.board.Columns) {
		visible = len(m.board.Columns)
	}
	return visible
}

func (m *Model) distributeWidth(numCols int) (baseWidth, remainder int) {
	if numCols == 0 || m.width == 0 {
		return minColumnWidth, 0
	}
	// lipgloss Width() includes padding, so only border (2) and margin (1) are outside
	borders := numCols * 2
	margins := numCols - 1
	available := m.width - borders - margins
	baseWidth = available / numCols
	remainder = available % numCols
	if baseWidth < minColumnWidth {
		baseWidth = minColumnWidth
		remainder = 0
	}
	return baseWidth, remainder
}

func (m *Model) moveTicket(delta int) {
	if len(m.columnTickets) <= m.activeColumn {
		return
	}
	tickets := m.columnTickets[m.activeColumn]
	m.activeTicket += delta
	if m.activeTicket < 0 {
		m.activeTicket = 0
	}
	if m.activeTicket >= len(tickets) {
		m.activeTicket = len(tickets) - 1
		if m.activeTicket < 0 {
			m.activeTicket = 0
		}
	}
}

// Action implementations
func (m *Model) createNewTicket() (tea.Model, tea.Cmd) {
	m.mode = ModeCreateTicket
	m.titleInput.Reset()
	m.titleInput.Focus()
	return m, m.titleInput.Cursor.BlinkCmd()
}

func (m *Model) attachToAgent() (tea.Model, tea.Cmd) {
	ticket := m.selectedTicket()
	if ticket == nil {
		m.notify("No ticket selected")
		return m, nil
	}

	pane, ok := m.panes[ticket.ID]
	if !ok || !pane.Running() {
		m.notify("No active agent for this ticket")
		return m, nil
	}

	m.mode = ModeAgentView
	m.focusedPane = ticket.ID
	paneHeight := m.height - 2
	pane.SetSize(m.width, paneHeight)
	return m, nil
}

func (m *Model) confirmDeleteTicket() (tea.Model, tea.Cmd) {
	ticket := m.selectedTicket()
	if ticket == nil {
		return m, nil
	}

	m.showConfirm = true
	m.confirmMsg = "Delete ticket: " + ticket.Title + "?"
	m.confirmFn = func() tea.Cmd {
		m.board.DeleteTicket(ticket.ID)
		m.refreshColumnTickets()
		m.saveBoard()
		m.notify("Deleted ticket")
		return nil
	}
	return m, nil
}

func (m *Model) quickMoveTicket() (tea.Model, tea.Cmd) {
	ticket := m.selectedTicket()
	if ticket == nil {
		return m, nil
	}

	// Move to next column
	nextStatus := m.nextStatus(ticket.Status)
	if nextStatus == ticket.Status {
		return m, nil
	}

	m.board.MoveTicket(ticket.ID, nextStatus)
	m.refreshColumnTickets()
	m.saveBoard()
	m.notify("Moved to " + string(nextStatus))

	return m, nil
}

func (m *Model) spawnAgent() (tea.Model, tea.Cmd) {
	ticket := m.selectedTicket()
	if ticket == nil {
		return m, nil
	}

	if ticket.Status != board.StatusInProgress {
		m.notify("Move ticket to In Progress first")
		return m, nil
	}

	if _, exists := m.panes[ticket.ID]; exists {
		m.notify("Agent already running")
		return m, nil
	}

	if ticket.WorktreePath == "" {
		branch := m.board.Settings.BranchPrefix + string(ticket.ID)[:8]
		baseBranch, _ := m.worktreeMgr.GetDefaultBranch()

		path, err := m.worktreeMgr.CreateWorktree(branch, baseBranch)
		if err != nil {
			m.notify("Failed to create worktree: " + err.Error())
			return m, nil
		}

		ticket.WorktreePath = path
		ticket.BranchName = branch
		ticket.BaseBranch = baseBranch
	}

	agentType := m.board.Settings.DefaultAgent
	agentCfg, ok := m.config.Agents[agentType]
	if !ok {
		m.notify("Unknown agent: " + agentType)
		return m, nil
	}

	pane := terminal.New(string(ticket.ID), m.width, m.height-2)
	pane.SetWorkdir(ticket.WorktreePath)
	m.panes[ticket.ID] = pane

	ticket.AgentType = agentType
	ticket.AgentStatus = board.AgentIdle

	m.saveBoard()
	m.notify("Starting " + agentType)

	m.mode = ModeAgentView
	m.focusedPane = ticket.ID

	return m, pane.Start(agentCfg.Command, agentCfg.Args...)
}

func (m *Model) stopAgent() (tea.Model, tea.Cmd) {
	ticket := m.selectedTicket()
	if ticket == nil {
		return m, nil
	}

	if pane, ok := m.panes[ticket.ID]; ok {
		pane.Stop()
		delete(m.panes, ticket.ID)
	}

	ticket.AgentStatus = board.AgentNone
	m.saveBoard()
	m.notify("Agent stopped")
	return m, nil
}

// Helper methods
func (m *Model) selectedTicket() *board.Ticket {
	if len(m.columnTickets) <= m.activeColumn {
		return nil
	}
	tickets := m.columnTickets[m.activeColumn]
	if len(tickets) <= m.activeTicket {
		return nil
	}
	return tickets[m.activeTicket]
}

func (m *Model) refreshColumnTickets() {
	m.columnTickets = make([][]*board.Ticket, len(m.board.Columns))
	for i, col := range m.board.Columns {
		m.columnTickets[i] = m.board.GetTicketsByStatus(col.Status)
	}
}

func (m *Model) nextStatus(current board.TicketStatus) board.TicketStatus {
	switch current {
	case board.StatusBacklog:
		return board.StatusInProgress
	case board.StatusInProgress:
		return board.StatusDone
	default:
		return current
	}
}

func (m *Model) notify(msg string) {
	m.notification = msg
	m.notifyTime = time.Now()
}

func (m *Model) saveBoard() {
	if err := m.board.Save(m.boardDir); err != nil {
		m.notify("Failed to save: " + err.Error())
	}
}

func (m *Model) handleTerminalMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, pane := range m.panes {
		if cmd := pane.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

type agentStatusMsg time.Time
type notificationMsg time.Time

func tickAgentStatus(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return agentStatusMsg(t)
	})
}
