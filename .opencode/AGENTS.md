# AGENTS.md - OpenKanban

TUI kanban board for orchestrating AI coding agents. Go 1.25+, Bubbletea, Lipgloss.

## Commands

```bash
go build ./...                      # Build
go test ./...                       # All tests
go test ./internal/config/...       # Single package
go test -run TestName ./...         # Single test
go vet ./...                        # Lint
```

## Architecture

**Bubbletea (Elm)**: Never block `Update()`. All I/O returns `tea.Cmd` for async execution.

**PTY Integration**: Embedded terminals via `creack/pty` + `hinshun/vt10x`. No tmux.

## Packages

| Package | Purpose |
|---------|---------|
| `ui/` | Bubbletea model, view, update cycle |
| `project/` | Project/ticket data, persistence, filtering |
| `agent/` | Agent config, status detection, context injection |
| `terminal/` | PTY-based terminal panes |
| `git/` | Worktree creation/removal |
| `config/` | Config loading from `~/.config/openkanban/config.json` |

## Key Files

- `ui/model.go` - Main state + Update (heart of app)
- `ui/view.go` - All rendering
- `project/project.go` - Project/ticket types
- `config/config.go` - Config types + defaults

## Style

- **Imports**: stdlib, external, internal (blank lines between)
- **Errors**: Return last, wrap with `fmt.Errorf`
- **Config**: All behavior must be configurable

## Do Not

- Block in `Update()` - return `tea.Cmd`
- Use tmux - embedded PTYs only
- Add AI attribution to commits
- Assume config exists - use defaults

## Commits

Conventional: `feat:`, `fix:`, `refactor:`, `perf:`, `docs:`, `test:`, `chore:`
