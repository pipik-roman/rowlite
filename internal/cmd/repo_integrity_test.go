package cmd_test

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRepositoryHermeticIntegrity verifies that all repository files are hermetically
// self-contained: no absolute local machine paths, no local file:/// URIs, and no
// markdown links that escape the repository root or reference non-existent local files.
func TestRepositoryHermeticIntegrity(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to locate repository root: %v", err)
	}

	// Generic patterns detecting developer machine workspace paths or local file URIs
	forbiddenPathPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)file:///[a-z]:`),                                                              // Local file URI scheme (file:///c:/...)
		regexp.MustCompile(`(?i)[a-z]:[\\/](?:work|users|home|workspace|projects|src|code|dev|repo)[\\/]`), // Windows developer workspace path
		regexp.MustCompile(`(?i)/Users/[a-z0-9_.-]+[\\/]`),                                                   // macOS user path (/Users/username/...)
		regexp.MustCompile(`(?i)/home/[a-z0-9_.-]+[\\/]`),                                                    // Linux user path (/home/username/...)
	}

	markdownLinkRegex := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

	scannedFiles := 0
	violations := 0

	err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip directories starting with '.' (.git, .agents, .gemini, .idea, .vscode) and vendor/dist/bin/scratch
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "bin" || name == "dist" || name == "scratch" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only inspect code, documentation, and configuration files
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".md", ".py", ".yaml", ".yml", ".json", ".sh", ".ps1":
			// Process
		default:
			return nil
		}

		// Skip the test file itself from matching its own pattern literals
		if strings.HasSuffix(path, "repo_integrity_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			t.Errorf("failed to open file %s: %v", relPath, err)
			return nil
		}
		defer file.Close()

		scannedFiles++
		fileDir := filepath.Dir(path)
		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// 1. Generic check for absolute machine paths
			for _, re := range forbiddenPathPatterns {
				if re.MatchString(line) {
					t.Errorf("Absolute machine path leaked in %s:%d:\n    %s", relPath, lineNum, strings.TrimSpace(line))
					violations++
				}
			}

			// 2. Generic check for markdown links escaping repository
			if ext == ".md" {
				matches := markdownLinkRegex.FindAllStringSubmatch(line, -1)
				for _, match := range matches {
					if len(match) < 2 {
						continue
					}
					target := strings.TrimSpace(match[1])

					// Ignore web URLs, anchors, mailto, and documentation placeholders
					if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
						strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") ||
						strings.Contains(target, "<") || strings.Contains(target, ">") {
						continue
					}

					// Parse target, strip query or fragment
					parsed, err := url.Parse(target)
					targetClean := target
					if err == nil && parsed.Path != "" {
						targetClean = parsed.Path
					}

					// Disallow file: URI schemes in links
					if strings.HasPrefix(target, "file:") {
						t.Errorf("Local file URI in markdown link %s:%d: %s", relPath, lineNum, target)
						violations++
						continue
					}

					// Relative link resolution
					resolvedTarget := filepath.Clean(filepath.Join(fileDir, targetClean))
					relToRepo, err := filepath.Rel(repoRoot, resolvedTarget)
					if err != nil || strings.HasPrefix(relToRepo, "..") {
						t.Errorf("Markdown link escapes repository root in %s:%d: %s", relPath, lineNum, target)
						violations++
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			t.Errorf("scanner error in %s: %v", relPath, err)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("filepath.Walk failed: %v", err)
	}

	t.Logf("Hermetic Integrity Guard: scanned %d files across repository. Violations: %d", scannedFiles, violations)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
