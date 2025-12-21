# OpenKanban Demo
#
# Dependencies: vhs, ffmpeg, ttyd (install ffmpeg and ttyd manually)
# Run with: VHS_NO_SANDBOX=true vhs docs/assets/demo.tape
#
# NOTE: VHS doesn't support Ctrl+S, so we use Ctrl+D to submit forms.
# The actual app uses Ctrl+S - the demo.gif shows correct UI hints.
#
# WARNING: This script deletes .openkanban/tickets.json to create a clean demo.
# Do not run in a project with real tickets you want to keep.

Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set Padding 20

# Clear existing tickets for a fresh demo
Hide
Type "rm -f .openkanban/tickets.json"
Enter
Type "clear"
Enter
Sleep 300ms
Show

# Launch OpenKanban
Type "./openkanban"
Sleep 300ms
Enter
Sleep 2s

# Create first ticket
Type "n"
Sleep 600ms
Type "Add user authentication"
Sleep 400ms
Tab
Sleep 400ms
Type "Implement JWT-based auth with refresh tokens"
Sleep 400ms
Ctrl+d
Sleep 1s

# Create second ticket
Type "n"
Sleep 600ms
Type "Setup database schema"
Sleep 400ms
Tab
Sleep 400ms
Type "PostgreSQL with migrations"
Sleep 400ms
Ctrl+d
Sleep 1s

# Navigate to first ticket and move to In Progress
Type "k"
Sleep 400ms
Type " "
Sleep 800ms

# Spawn agent (ticket stays selected after move)
Type "s"
Sleep 9s

# Detach quickly while agent is still running
Ctrl+g
Sleep 2s

# Create third ticket while agent runs in background
Type "n"
Sleep 600ms
Type "Build REST API endpoints"
Sleep 400ms
Tab
Sleep 400ms
Type "CRUD operations for all resources"
Sleep 400ms
Ctrl+d
Sleep 2s


# Spawn agent on second ticket
Type "s"
Sleep 9s

# Detach quickly while agent is still running
Ctrl+g
Sleep 2s

# Navigate back to the first ticket (In Progress column, go up)
Type "k"
Sleep 2s

# Re-attach to show the original agent is still running
Enter
Sleep 2s
Type "You're doing great, keep going!"
Enter
Sleep 4s

# Detach one more time to show the board with agents still in progress
Ctrl+g
Sleep 2s

# Navigate back to the original session and type a message
Type "j"
Sleep 2s
Enter
Sleep 4s

# Back to main board
Ctrl+g
Sleep 3s

# Quit
Type "q"
Sleep 500ms
