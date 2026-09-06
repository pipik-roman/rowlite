Output docs/assets/rowlite-demo.gif

Set Shell "bash"
Set FontSize 16
Set FontFamily "JetBrains Mono"
Set Width 950
Set Height 520
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 65ms
Set Framerate 15

Hide
Type "alias rowlite='./rowlite'" Enter
Type "export PS1='❯ '" Enter
Type "rm -f tasks.md" Enter
Type "clear" Enter
Show

Type "# Step 1: Initialize table schema with autoincrement ID & audit logging" Enter
Sleep 800ms
Type "rowlite init tasks.md --columns 'id,task,status,priority' --autoincrement 'id' --audit" Enter
Sleep 2000ms

Type "# Step 2: Append rows without manual ID or timestamp management" Enter
Sleep 800ms
Type "rowlite append tasks.md ',Implement MCP protocol,pending,high'" Enter
Sleep 1500ms
Type "rowlite append tasks.md ',Record hero demo GIF,in_progress,medium'" Enter
Sleep 2000ms

Type "# Step 3: Query table with clean Unicode formatting" Enter
Sleep 800ms
Type "rowlite list tasks.md --format pretty" Enter
Sleep 3500ms

Type "# Step 4: Precisely update row fields in-place" Enter
Sleep 800ms
Type "rowlite update tasks.md --where 'id == 1' --set 'status=completed'" Enter
Sleep 2000ms

Type "# Step 5: Verify auto-formatted Markdown table on disk" Enter
Sleep 800ms
Type "cat tasks.md" Enter
Sleep 4500ms
