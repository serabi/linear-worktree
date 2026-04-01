# Features demo: fuzzy search, filter cycling, detail view
Output demo/gif/features.gif
Set Theme "Catppuccin Mocha"
Set Width 1200
Set Height 600
Set FontSize 14
Set Padding 20

Hide
Type "./linear-worktree --demo"
Enter
Sleep 2s
Show

Sleep 1s

# Use fuzzy filter to find issues
Type "/"
Sleep 500ms
Type "rate"
Sleep 1500ms
Escape
Sleep 1s

# Cycle through all filter modes
Tab
Sleep 800ms
Tab
Sleep 800ms
Tab
Sleep 800ms
Tab
Sleep 800ms
Tab
Sleep 800ms

Sleep 1s

# Navigate to an issue with labels
Down
Sleep 400ms
Down
Sleep 400ms

# Open detail view
Type "d"
Sleep 3s

# Close detail
Escape
Sleep 1s

Sleep 2s
