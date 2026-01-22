package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolver_ResolveName_VersionedPath(t *testing.T) {
	r := New()

	name := r.ResolveName("gopkg.in/yaml.v3")

	assert.Equal(t, "yaml", name)
}

func TestResolver_ResolveName_StandardLibrary(t *testing.T) {
	r := New()

	tests := []struct {
		importPath string
		expected   string
	}{
		{"fmt", "fmt"},
		{"net/http", "http"},
		{"encoding/json", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.importPath, func(t *testing.T) {
			name := r.ResolveName(tt.importPath)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestResolver_ResolveName_Caching(t *testing.T) {
	r := New()

	name1 := r.ResolveName("fmt")
	name2 := r.ResolveName("fmt")

	assert.Equal(t, name1, name2)
}

func TestResolver_ResolveName_UnknownPackageFallsBackWithVersionStripped(t *testing.T) {
	r := New()

	name := r.ResolveName("github.com/nonexistent/package/v2")

	assert.Equal(t, "package", name)
}

func TestFallbackName(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{
			name:       "versioned path v2",
			importPath: "github.com/example/pkg/v2",
			expected:   "pkg",
		},
		{
			name:       "versioned path v10",
			importPath: "github.com/example/pkg/v10",
			expected:   "pkg",
		},
		{
			name:       "echo v4",
			importPath: "github.com/labstack/echo/v4",
			expected:   "echo",
		},
		{
			name:       "chi v5",
			importPath: "github.com/go-chi/chi/v5",
			expected:   "chi",
		},
		{
			name:       "non-versioned path",
			importPath: "github.com/example/pkg",
			expected:   "pkg",
		},
		{
			name:       "standard library",
			importPath: "fmt",
			expected:   "fmt",
		},
		{
			name:       "gopkg.in style yaml.v3",
			importPath: "gopkg.in/yaml.v3",
			expected:   "yaml",
		},
		{
			name:       "gopkg.in style check.v1",
			importPath: "gopkg.in/check.v1",
			expected:   "check",
		},
		{
			name:       "path ending with v but not version",
			importPath: "github.com/example/v",
			expected:   "v",
		},
		{
			name:       "path with .v in middle",
			importPath: "github.com/example/foo.v2.bar",
			expected:   "foo.v2.bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := fallbackName(tt.importPath)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestIsVersionSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"v2", true},
		{"v10", true},
		{"v0", true},
		{"v", false},
		{"va", false},
		{"2v", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isVersionSuffix(tt.input))
		})
	}
}

func TestVersionSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"yaml.v3", ".v3"},
		{"check.v1", ".v1"},
		{"pkg.v10", ".v10"},
		{"pkg", ""},
		{"pkg.va", ""},
		{"foo.v2.bar", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, versionSuffix(tt.input))
		})
	}
}
