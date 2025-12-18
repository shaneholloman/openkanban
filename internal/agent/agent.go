package agent

import (
	"time"

	"github.com/techdufus/openkanban/internal/board"
	"github.com/techdufus/openkanban/internal/config"
)

// Manager handles AI agent configuration and status polling.
// Agent lifecycle (spawn/stop) is now managed by terminal.Pane via PTY.
type Manager struct {
	config *config.Config
}

// NewManager creates a new agent manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{config: cfg}
}

// GetAgentConfig returns the configuration for a specific agent type
func (m *Manager) GetAgentConfig(agentType string) (*config.AgentConfig, bool) {
	cfg, ok := m.config.Agents[agentType]
	return &cfg, ok
}

// PollStatuses is a no-op placeholder.
// Agent status is now tracked by the UI via terminal.Pane.Running().
func (m *Manager) PollStatuses(tickets map[board.TicketID]*board.Ticket) {
	// Status is now managed by terminal panes, not tmux sessions.
	// This method is kept for interface compatibility but does nothing.
}

// StatusPollInterval returns the configured polling interval
func (m *Manager) StatusPollInterval() time.Duration {
	interval := m.config.UI.RefreshInterval
	if interval <= 0 {
		interval = 5
	}
	return time.Duration(interval) * time.Second
}
