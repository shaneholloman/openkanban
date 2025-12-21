# OpenKanban

A TUI kanban board for orchestrating AI coding agents.

<p align="center">
  <img src="./docs/assets/demo.gif" alt="OpenKanban Demo" />
</p>

## Why?

AI coding agents are powerful, but managing multiple agents across projects gets messy fast. You end up with terminals everywhere, losing track of what's running where, and context-switching between tasks becomes a chore.

OpenKanban gives you a single view of all your work. Each ticket gets its own git worktree and embedded terminal. Spawn an agent, watch it work, jump between tasks. Everything stays organized.

## What It Does

- **Tickets as worktrees** - Each task gets an isolated git branch
- **Embedded terminals** - Agents run inside the TUI, not in random terminal tabs
- **Any agent** - OpenCode, Claude Code, Aider, or whatever CLI tool you prefer
- **Multi-project** - Manage tickets across all your repositories from one board

## Install

```bash
go install github.com/techdufus/openkanban@latest
```

## Quick Start

```bash
cd ~/projects/my-app
openkanban new "My App"
openkanban
```

## Configuration

OpenKanban is designed to be highly configurable. Agents, keybindings, branch naming, cleanup behavior - all customizable in `~/.config/openkanban/config.json`.

See [Configuration Guide](./docs/CONFIGURATION.md) for details.

## Keybindings

| Key | Action |
|-----|--------|
| `n` | New ticket |
| `s` | Spawn agent |
| `enter` | Attach to agent |
| `h/l` | Move ticket between columns |
| `?` | Full help |

## License

MIT
