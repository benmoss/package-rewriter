package rewriter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// GoModManager handles reading and writing go.mod files
type GoModManager struct {
	path    string
	file    *modfile.File
	content []byte
}

// NewGoModManager creates a new go.mod manager
func NewGoModManager(path string) (*GoModManager, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	return &GoModManager{
		path:    path,
		file:    file,
		content: content,
	}, nil
}

// HasReplace checks if a replace directive exists for the given module path
func (m *GoModManager) HasReplace(modulePath string) bool {
	for _, replace := range m.file.Replace {
		if replace.Old.Path == modulePath {
			return true
		}
	}
	return false
}

// RemoveReplace removes a replace directive for the given module path
func (m *GoModManager) RemoveReplace(modulePath string) error {
	return m.file.DropReplace(modulePath, "")
}

// AddReplace adds a replace directive
func (m *GoModManager) AddReplace(modulePath, localPath string) error {
	return m.file.AddReplace(modulePath, "", localPath, "")
}

// Save writes the modified go.mod back to disk
func (m *GoModManager) Save() error {
	formatted, err := m.file.Format()
	if err != nil {
		return fmt.Errorf("failed to format go.mod: %w", err)
	}

	if err := os.WriteFile(m.path, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	return nil
}

// GetReplaces returns all replace directives as a map
func (m *GoModManager) GetReplaces() map[string]string {
	replaces := make(map[string]string)
	for _, replace := range m.file.Replace {
		replaces[replace.Old.Path] = replace.New.Path
	}
	return replaces
}

// Tidy runs 'go mod tidy' in the directory containing the go.mod file
func (m *GoModManager) Tidy() error {
	dir := filepath.Dir(m.path)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// FindGoMod finds the go.mod file starting from the current directory
func FindGoMod() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Look for go.mod in current directory and parent directories
	for {
		goModPath := dir + "/go.mod"
		if _, err := os.Stat(goModPath); err == nil {
			return goModPath, nil
		}

		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("go.mod not found in current directory or any parent directory")
}
