package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InitAgletRepo creates and initializes a .aglet/ directory inside the given
// domain directory. Sets up a git repo with a .gitignore that excludes log files.
// This is the runtime data store for all units in the domain.
func InitAgletRepo(domainDir string) error {
	agletDir := filepath.Join(domainDir, ".aglet")

	// Create directory
	if err := os.MkdirAll(agletDir, 0755); err != nil {
		return fmt.Errorf("could not create .aglet/: %w", err)
	}

	// Write .gitignore — logs are working data, only memory.json is committed
	gitignorePath := filepath.Join(agletDir, ".gitignore")
	gitignoreContent := "# Aglet runtime data — only memory.json files are committed\n**/logs.jsonl\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("could not write .aglet/.gitignore: %w", err)
	}

	// Initialize git repo (best-effort — if git isn't installed, warn but don't fail)
	cmd := exec.Command("git", "init")
	cmd.Dir = agletDir
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[aglet] Warning: could not initialize git in .aglet/: %s\n", err)
		return nil // non-fatal
	}

	// Initial commit with .gitignore
	addCmd := exec.Command("git", "add", ".gitignore")
	addCmd.Dir = agletDir
	addCmd.Run() // best-effort

	commitCmd := exec.Command("git", "commit", "-m", "Initialize .aglet/ runtime data repo")
	commitCmd.Dir = agletDir
	commitCmd.Stdout = nil
	commitCmd.Stderr = nil
	commitCmd.Run() // best-effort

	return nil
}

// SnapshotAgletRepo commits all memory.json files in the .aglet/ repo,
// referencing the main repo's current HEAD commit for correlation.
func SnapshotAgletRepo(domainDir string) error {
	agletDir := filepath.Join(domainDir, ".aglet")

	// Check .aglet/ exists
	if _, err := os.Stat(agletDir); os.IsNotExist(err) {
		return nil // no .aglet/ directory, nothing to snapshot
	}

	// Check if .aglet/ has a git repo
	if _, err := os.Stat(filepath.Join(agletDir, ".git")); os.IsNotExist(err) {
		return nil // no git repo, nothing to snapshot
	}

	// Get main repo HEAD for the commit message
	mainHead := getMainRepoHead(domainDir)

	// Stage all memory.json files
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = agletDir
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed in .aglet/: %w", err)
	}

	// Check if there's anything to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = agletDir
	statusOutput, _ := statusCmd.Output()
	if len(strings.TrimSpace(string(statusOutput))) == 0 {
		return nil // nothing to commit
	}

	// Commit with reference to main repo
	msg := fmt.Sprintf("Behavioral snapshot")
	if mainHead != "" {
		msg = fmt.Sprintf("Behavioral snapshot for %s", mainHead)
	}
	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = agletDir
	commitCmd.Stdout = nil
	commitCmd.Stderr = nil
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed in .aglet/: %w", err)
	}

	return nil
}

// EnsureAgletDir creates the .aglet/{unitName}/ directory if it doesn't exist.
// Called lazily on first write — no upfront setup required.
func EnsureAgletDir(agletUnitDir string) error {
	return os.MkdirAll(agletUnitDir, 0755)
}

// getMainRepoHead returns the short commit hash of the main repo's HEAD.
// Returns "" if git fails or there are no commits.
func getMainRepoHead(domainDir string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = domainDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// handleSnapshot is the CLI handler for `aglet snapshot`.
// Commits memory files in all .aglet/ repos across the project.
func handleSnapshot() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot get working directory: %s\n", err)
		os.Exit(1)
	}

	// Find project root
	projectRoot := FindProjectRoot(cwd)
	if projectRoot == "" {
		fmt.Fprintf(os.Stderr, "Error: not inside an Aglet project (no domain.yaml found)\n")
		os.Exit(1)
	}

	// Walk the project tree to find all domain directories
	domains := findAllDomainDirs(projectRoot)
	if len(domains) == 0 {
		fmt.Fprintf(os.Stderr, "[aglet snapshot] No domains found\n")
		return
	}

	snapped := 0
	for _, domainDir := range domains {
		agletDir := filepath.Join(domainDir, ".aglet")
		if _, err := os.Stat(agletDir); os.IsNotExist(err) {
			continue
		}

		if err := SnapshotAgletRepo(domainDir); err != nil {
			fmt.Fprintf(os.Stderr, "[aglet snapshot] Warning: %s: %s\n", domainDir, err)
			continue
		}

		// Check if anything was actually committed
		relDir, _ := filepath.Rel(projectRoot, domainDir)
		if relDir == "." {
			relDir = "(root)"
		}
		fmt.Fprintf(os.Stderr, "[aglet snapshot] %s/.aglet/ — committed\n", relDir)
		snapped++
	}

	if snapped == 0 {
		fmt.Fprintf(os.Stderr, "[aglet snapshot] Nothing to commit (no changes in memory files)\n")
	}
}

// findAllDomainDirs walks the project tree and returns all directories containing domain.yaml.
func findAllDomainDirs(projectRoot string) []string {
	var dirs []string
	filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories and node_modules
		if info.IsDir() && (strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if info.Name() == "domain.yaml" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	return dirs
}

// FindProjectRoot walks up from dir looking for a root domain.yaml (one without a parent field).
// Falls back to the first domain.yaml found if none is a root.
func FindProjectRoot(dir string) string {
	dir, _ = filepath.Abs(dir)
	current := dir
	for {
		domainPath := filepath.Join(current, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err == nil && domain.Parent == "" {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "" // reached filesystem root
		}
		current = parent
	}
}
