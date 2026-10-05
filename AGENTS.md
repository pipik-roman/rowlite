# Rowlite Development Guidelines

## Project Context
Rowlite is an embedded Markdown-first transactional and query engine written in Go. It operates directly on GitHub Flavored Markdown (GFM) tables with YAML frontmatter headers.

## Media & Visual Demos
* **GIF Format Only**: When asked to create visual demonstrations, terminal recordings, or user interface flows (even if the prompt uses words like "video", "screencast", or "recording"), agents MUST generate animated `.gif` files (with static `.png` frame snapshots) only.
* **No Video Container Formats**: Agents MUST NEVER generate video container formats (`.mp4`, `.webm`, `.avi`, `.mov`) in this repository. All recorded terminal assets must reside in `docs/assets/` as `.gif` or `.png`.

## Code & Quality Standards
* Follow standard Go idioms and project architecture (`internal/core/` for storage/table engine, `internal/cmd/` for CLI commands).
* Ensure all tests pass (`go test ./...`) before completing any feature work.
