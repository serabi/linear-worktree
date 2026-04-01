# Hero demo: browse issues, open detail panel with comments
Output demo/gif/hero.gif
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

Sleep 2s

# Navigate down through the issue list
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 600ms

# Back up to first issue
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms

Sleep 1s

# Open detail panel on first issue (rate limiting)
Type "d"
Sleep 3s

# Scroll through description and comments
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms

Sleep 2s

# Close detail
Escape
Sleep 1s

# Cycle through filters
Tab
Sleep 1s
Tab
Sleep 1s
Tab
Sleep 1s

Sleep 2s
