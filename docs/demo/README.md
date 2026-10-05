# Rowlite Hero Terminal Demo

This directory contains the declarative Charmbracelet VHS recording script for generating the Rowlite hero demonstration GIF embedded in `README.md`.

## Tape Source Specification
- **Script**: `hero.tape`
- **Output Target**: `../assets/rowlite-demo.gif`
- **Canvas Geometry**:
  - **Width**: `950` px
  - **Height**: `520` px
  - **Padding**: `20` px
  - **Corner Radius**: `10` px
  - **Font Size**: `16` px
  - **Font Family**: `"JetBrains Mono"`
- **Theme**: `"Catppuccin Mocha"`
- **Playback & Timing**:
  - **Framerate**: `15` FPS (guarantees lightweight GIF output <= 3.5 MB)
  - **Typing Speed**: `45ms`
  - **Command Pauses**: `600ms` pre-command, `1200ms`–`2500ms` output display, `3500ms` loop boundary

## Demonstrated Command Sequence

1. **Table Initialization**:
   ```bash
   rowlite init tasks.md --columns "id,task,status,priority" --autoincrement "id" --audit
   ```
2. **Row Append (Auto-Increment & Audit Timestamps)**:
   ```bash
   rowlite append tasks.md ',Implement MCP protocol,pending,high'
   rowlite append tasks.md ',Record hero demo GIF,in_progress,medium'
   ```
3. **Query & Terminal Formatting**:
   ```bash
   rowlite list tasks.md --format pretty
   ```
4. **In-Place Update**:
   ```bash
   rowlite update tasks.md --where 'id == 1' --set 'status=completed'
   ```
5. **Disk File Verification**:
   ```bash
   cat tasks.md
   ```

## Rendering the Demo GIF

### Using Local VHS CLI
```bash
# Ensure rowlite binary is built
go build -o rowlite ./cmd/rowlite

# Render GIF
vhs docs/demo/hero.tape
```

### Using Docker Container (Zero-Install)
```bash
docker run --rm -v "$PWD:/vhs" ghcr.io/charmbracelet/vhs docs/demo/hero.tape
```

### Optional Size Optimization via Gifsicle
```bash
gifsicle -O3 --lossy=80 docs/assets/rowlite-demo.gif -o docs/assets/rowlite-demo.gif
```

## Media Formats & Repository Policy
- **GIF Only**: All terminal recording scripts in `docs/demo/` must output exclusively to `.gif` (and optional static frame `.png` snapshots).
- **No Video Formats**: Do not generate or check in `.mp4`, `.webm`, or other binary video container formats.

