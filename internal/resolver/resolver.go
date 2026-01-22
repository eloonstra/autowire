package resolver

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eloonstra/autowire/internal/xsync"
)

const goListOutputParts = 2

type Resolver struct {
	cache xsync.Map[string, string]
}

func New() *Resolver {
	return &Resolver{}
}

func (r *Resolver) ResolveName(importPath string) string {
	if name, ok := r.cache.Load(importPath); ok {
		return name
	}

	name := r.resolve(importPath)
	actual, _ := r.cache.LoadOrStore(importPath, name)
	return actual
}

func (r *Resolver) resolve(path string) string {
	cmd := exec.Command("go", "list", "-e", "-f", "{{.ImportPath}} {{.Name}}", path)
	out, err := cmd.Output()
	if err != nil {
		return fallbackName(path)
	}

	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, " ", goListOutputParts)
	if len(parts) != goListOutputParts {
		return fallbackName(path)
	}

	return parts[1]
}

func fallbackName(importPath string) string {
	base := filepath.Base(importPath)
	if isVersionSuffix(base) {
		return filepath.Base(filepath.Dir(importPath))
	}
	if name, ok := strings.CutSuffix(base, versionSuffix(base)); ok {
		return name
	}
	return base
}

func isVersionSuffix(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func versionSuffix(s string) string {
	idx := strings.LastIndex(s, ".v")
	if idx == -1 {
		return ""
	}
	suffix := s[idx+1:]
	if !isVersionSuffix(suffix) {
		return ""
	}
	return "." + suffix
}
