# Configuration Guide

OpenKanban configuration lives in `~/.config/openkanban/config.json`.

## Default Configuration

```json
{
  "defaults": {
    "default_agent": "opencode",
    "branch_prefix": "task/",
    "branch_template": "{prefix}{slug}",
    "slug_max_length": 40,
    "auto_spawn_agent": true,
    "auto_create_branch": true
  },
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": []
    },
    "claude": {
      "command": "claude",
      "args": ["--dangerously-skip-permissions"]
    },
    "aider": {
      "command": "aider",
      "args": ["--yes"]
    }
  },
  "ui": {
    "theme": "catppuccin-mocha",
    "show_agent_status": true,
    "refresh_interval": 5,
    "column_width": 40,
    "ticket_height": 4
  },
  "cleanup": {
    "delete_worktree": true,
    "delete_branch": false,
    "force_worktree_removal": false
  }
}
```

## Agents

Define any CLI-based agent. The command runs in the ticket's worktree directory.

```json
{
  "agents": {
    "my-agent": {
      "command": "my-agent-cli",
      "args": ["--flag", "value"],
      "env": {
        "CUSTOM_VAR": "value"
      },
      "init_prompt": "Custom prompt template with {{.Title}} and {{.Description}}"
    }
  }
}
```

### Init Prompt Variables

When spawning an agent, OpenKanban can inject ticket context:

- `{{.Title}}` - Ticket title
- `{{.Description}}` - Ticket description
- `{{.BranchName}}` - Git branch name
- `{{.BaseBranch}}` - Base branch (e.g., main)

## Branch Naming

Control how branches are named:

```json
{
  "defaults": {
    "branch_prefix": "feature/",
    "branch_template": "{prefix}{slug}",
    "slug_max_length": 30
  }
}
```

A ticket titled "Add user authentication" becomes branch `feature/add-user-authentication`.

## Cleanup Behavior

When deleting tickets:

```json
{
  "cleanup": {
    "delete_worktree": true,
    "delete_branch": false,
    "force_worktree_removal": false
  }
}
```

- `delete_worktree` - Remove the git worktree directory
- `delete_branch` - Also delete the git branch
- `force_worktree_removal` - Force removal even with uncommitted changes

## Keybindings

All keybindings are shown in-app with `?`. Custom keybindings coming soon.

## Full Keybindings Reference

### Board View

| Key | Action |
|-----|--------|
| `j/k` | Move cursor up/down |
| `h/l` | Move between columns |
| `space` | Move ticket to next column |
| `-` | Move ticket to previous column |
| `enter` | Attach to running agent |
| `n` | Create new ticket |
| `e` | Edit ticket |
| `s` | Spawn agent for ticket |
| `S` | Stop agent |
| `d` | Delete ticket |
| `p` | Cycle project filter |
| `?` | Show help |
| `q` | Quit |

### Agent View

| Key | Action |
|-----|--------|
| `ctrl+g` | Return to board |
| All other keys | Passed to agent |
