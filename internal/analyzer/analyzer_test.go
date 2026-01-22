package analyzer

import (
	"path/filepath"
	"testing"

	"github.com/eloonstra/autowire/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResolver struct{}

func (m *mockResolver) ResolveName(importPath string) string {
	return filepath.Base(importPath)
}

type versionedPathResolver struct{}

func (v *versionedPathResolver) ResolveName(importPath string) string {
	knownPackages := map[string]string{
		"github.com/go-chi/chi/v5": "chi",
		"gopkg.in/yaml.v3":         "yaml",
	}
	if name, ok := knownPackages[importPath]; ok {
		return name
	}
	return filepath.Base(importPath)
}

func TestAnalyze_DuplicateProvider(t *testing.T) {
	parsed := &types.ParseResult{
		Providers: []types.Provider{
			{
				Name:         "NewConfigA",
				Kind:         types.ProviderKindFunc,
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				VarName:      "config",
			},
			{
				Name:         "NewConfigB",
				Kind:         types.ProviderKindFunc,
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				VarName:      "config",
			},
		},
		OutputPackage:    "main",
		OutputImportPath: "example.com/app",
	}

	_, err := Analyze(parsed, &mockResolver{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate provider")
}

func TestAnalyze_Success(t *testing.T) {
	parsed := &types.ParseResult{
		Providers: []types.Provider{
			{
				Name:         "NewConfig",
				Kind:         types.ProviderKindFunc,
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				VarName:      "config",
			},
			{
				Name:         "NewDatabase",
				Kind:         types.ProviderKindFunc,
				ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg/db", IsPointer: true},
				Dependencies: []types.Dependency{
					{Type: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true}},
				},
				ImportPath: "pkg/db",
				VarName:    "database",
			},
		},
		OutputPackage:    "main",
		OutputImportPath: "example.com/app",
	}

	result, err := Analyze(parsed, &mockResolver{})
	require.NoError(t, err)
	assert.Equal(t, "main", result.PackageName)
	assert.Len(t, result.Providers, 2)
}

func TestValidateDeps(t *testing.T) {
	tests := []struct {
		name        string
		providers   []types.Provider
		invocations []types.Invocation
		wantErr     bool
		errContains string
	}{
		{
			name: "all deps satisfied",
			providers: []types.Provider{
				{
					Name:         "NewConfig",
					ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true},
				},
				{
					Name:         "NewDatabase",
					ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg", IsPointer: true},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing provider dependency",
			providers: []types.Provider{
				{
					Name:         "NewDatabase",
					ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg", IsPointer: true},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true}},
					},
				},
			},
			wantErr:     true,
			errContains: "missing dependencies",
		},
		{
			name: "missing invocation dependency",
			providers: []types.Provider{
				{
					Name:         "NewConfig",
					ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true},
				},
			},
			invocations: []types.Invocation{
				{
					Name: "Setup",
					Dependencies: []types.TypeRef{
						{Name: "Database", ImportPath: "pkg", IsPointer: true},
					},
				},
			},
			wantErr:     true,
			errContains: "missing dependencies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byType := make(map[string]types.Provider)
			for _, p := range tt.providers {
				byType[p.ProvidedType.Key()] = p
			}

			err := validateDeps(tt.providers, tt.invocations, byType)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestTopoSort(t *testing.T) {
	tests := []struct {
		name        string
		providers   []types.Provider
		invocations []types.Invocation
		checkOrder  func(t *testing.T, result []types.Provider)
	}{
		{
			name: "linear chain A->B->C",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "B", ImportPath: "pkg"}},
					},
					VarName: "a",
				},
				{
					Name:         "B",
					ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "C", ImportPath: "pkg"}},
					},
					VarName: "b",
				},
				{
					Name:         "C",
					ProvidedType: types.TypeRef{Name: "C", ImportPath: "pkg"},
					VarName:      "c",
				},
			},
			checkOrder: func(t *testing.T, result []types.Provider) {
				assert.Len(t, result, 3)
				indexC := indexOf(result, "C")
				indexB := indexOf(result, "B")
				indexA := indexOf(result, "A")
				assert.Less(t, indexC, indexB, "C should come before B")
				assert.Less(t, indexB, indexA, "B should come before A")
			},
		},
		{
			name: "diamond dependency",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "B", ImportPath: "pkg"}},
						{Type: types.TypeRef{Name: "C", ImportPath: "pkg"}},
					},
					VarName: "a",
				},
				{
					Name:         "B",
					ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "D", ImportPath: "pkg"}},
					},
					VarName: "b",
				},
				{
					Name:         "C",
					ProvidedType: types.TypeRef{Name: "C", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "D", ImportPath: "pkg"}},
					},
					VarName: "c",
				},
				{
					Name:         "D",
					ProvidedType: types.TypeRef{Name: "D", ImportPath: "pkg"},
					VarName:      "d",
				},
			},
			checkOrder: func(t *testing.T, result []types.Provider) {
				assert.Len(t, result, 4)
				indexD := indexOf(result, "D")
				indexB := indexOf(result, "B")
				indexC := indexOf(result, "C")
				indexA := indexOf(result, "A")
				assert.Less(t, indexD, indexB, "D should come before B")
				assert.Less(t, indexD, indexC, "D should come before C")
				assert.Less(t, indexB, indexA, "B should come before A")
				assert.Less(t, indexC, indexA, "C should come before A")
			},
		},
		{
			name: "independent providers",
			providers: []types.Provider{
				{Name: "A", ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"}, VarName: "a"},
				{Name: "B", ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"}, VarName: "b"},
				{Name: "C", ProvidedType: types.TypeRef{Name: "C", ImportPath: "pkg"}, VarName: "c"},
			},
			checkOrder: func(t *testing.T, result []types.Provider) {
				assert.Len(t, result, 3)
				names := make(map[string]bool)
				for _, p := range result {
					names[p.Name] = true
				}
				assert.True(t, names["A"])
				assert.True(t, names["B"])
				assert.True(t, names["C"])
			},
		},
		{
			name: "invocation triggers dependency",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "B", ImportPath: "pkg"}},
					},
					VarName: "a",
				},
				{Name: "B", ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"}, VarName: "b"},
			},
			invocations: []types.Invocation{
				{
					Name: "Setup",
					Dependencies: []types.TypeRef{
						{Name: "A", ImportPath: "pkg"},
					},
				},
			},
			checkOrder: func(t *testing.T, result []types.Provider) {
				assert.Len(t, result, 2)
				indexB := indexOf(result, "B")
				indexA := indexOf(result, "A")
				assert.Less(t, indexB, indexA, "B should come before A")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byType := make(map[string]types.Provider)
			for _, p := range tt.providers {
				byType[p.ProvidedType.Key()] = p
			}

			result, err := topoSort(tt.providers, tt.invocations, byType)
			require.NoError(t, err)

			if tt.checkOrder != nil {
				tt.checkOrder(t, result)
			}
		})
	}
}

func indexOf(providers []types.Provider, name string) int {
	for i, p := range providers {
		if p.Name == name {
			return i
		}
	}
	return -1
}

func TestTopoSort_CycleDetection(t *testing.T) {
	tests := []struct {
		name      string
		providers []types.Provider
		errMsg    string
	}{
		{
			name: "direct cycle A->B->A",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "B", ImportPath: "pkg"}},
					},
				},
				{
					Name:         "B",
					ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "A", ImportPath: "pkg"}},
					},
				},
			},
			errMsg: "circular dependency",
		},
		{
			name: "indirect cycle A->B->C->A",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "B", ImportPath: "pkg"}},
					},
				},
				{
					Name:         "B",
					ProvidedType: types.TypeRef{Name: "B", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "C", ImportPath: "pkg"}},
					},
				},
				{
					Name:         "C",
					ProvidedType: types.TypeRef{Name: "C", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "A", ImportPath: "pkg"}},
					},
				},
			},
			errMsg: "circular dependency",
		},
		{
			name: "self cycle A->A",
			providers: []types.Provider{
				{
					Name:         "A",
					ProvidedType: types.TypeRef{Name: "A", ImportPath: "pkg"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "A", ImportPath: "pkg"}},
					},
				},
			},
			errMsg: "circular dependency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byType := make(map[string]types.Provider)
			for _, p := range tt.providers {
				byType[p.ProvidedType.Key()] = p
			}

			_, err := topoSort(tt.providers, nil, byType)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestResolveVarNames(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no collision",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "two same",
			input:    []string{"config", "config"},
			expected: []string{"config", "config1"},
		},
		{
			name:     "three same",
			input:    []string{"cfg", "cfg", "cfg"},
			expected: []string{"cfg", "cfg1", "cfg2"},
		},
		{
			name:     "mixed",
			input:    []string{"a", "b", "a", "c", "a"},
			expected: []string{"a", "b", "a1", "c", "a2"},
		},
		{
			name:     "empty",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := make([]types.Provider, len(tt.input))
			for i, name := range tt.input {
				providers[i] = types.Provider{VarName: name}
			}

			resolveVarNames(providers)

			for i, expected := range tt.expected {
				assert.Equal(t, expected, providers[i].VarName)
			}
		})
	}
}

func TestCollectImports(t *testing.T) {
	const outputPath = "example.com/app"

	tests := []struct {
		name        string
		providers   []types.Provider
		invocations []types.Invocation
		expectPaths []string
		excludePath string
	}{
		{
			name:        "empty",
			providers:   []types.Provider{},
			invocations: []types.Invocation{},
			expectPaths: []string{},
		},
		{
			name: "skip output path",
			providers: []types.Provider{
				{
					ImportPath:   outputPath,
					ProvidedType: types.TypeRef{Name: "Config", ImportPath: outputPath},
				},
			},
			expectPaths: []string{},
			excludePath: outputPath,
		},
		{
			name: "collect provider path",
			providers: []types.Provider{
				{
					ImportPath:   "pkg/config",
					ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config"},
				},
			},
			expectPaths: []string{"pkg/config"},
		},
		{
			name: "collect dependency paths",
			providers: []types.Provider{
				{
					ImportPath:   "pkg/service",
					ProvidedType: types.TypeRef{Name: "Service", ImportPath: "pkg/service"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "Config", ImportPath: "pkg/config"}},
						{Type: types.TypeRef{Name: "Database", ImportPath: "pkg/db"}},
					},
				},
			},
			expectPaths: []string{"pkg/service", "pkg/config", "pkg/db"},
		},
		{
			name: "collect invocation paths",
			invocations: []types.Invocation{
				{
					ImportPath: "pkg/setup",
					Dependencies: []types.TypeRef{
						{Name: "Config", ImportPath: "pkg/config"},
					},
				},
			},
			expectPaths: []string{"pkg/setup", "pkg/config"},
		},
		{
			name: "skip empty import path",
			providers: []types.Provider{
				{
					ImportPath:   "pkg/service",
					ProvidedType: types.TypeRef{Name: "Service", ImportPath: "pkg/service"},
					Dependencies: []types.Dependency{
						{Type: types.TypeRef{Name: "string", ImportPath: ""}},
					},
				},
			},
			expectPaths: []string{"pkg/service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectImports(tt.providers, tt.invocations, outputPath, &mockResolver{})

			for _, path := range tt.expectPaths {
				_, exists := result[path]
				assert.True(t, exists, "expected path %s to be in imports", path)
			}

			if tt.excludePath != "" {
				_, exists := result[tt.excludePath]
				assert.False(t, exists, "expected path %s to NOT be in imports", tt.excludePath)
			}
		})
	}
}

func TestResolveImportAliases(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected map[string]string
	}{
		{
			name:     "unique bases",
			paths:    []string{"pkg/foo", "other/bar"},
			expected: map[string]string{"pkg/foo": "", "other/bar": ""},
		},
		{
			name:     "duplicate base",
			paths:    []string{"pkg/http", "other/http"},
			expected: map[string]string{"other/http": "", "pkg/http": "http1"},
		},
		{
			name:     "triple collision",
			paths:    []string{"a/foo", "b/foo", "c/foo"},
			expected: map[string]string{"a/foo": "", "b/foo": "foo1", "c/foo": "foo2"},
		},
		{
			name:     "empty",
			paths:    []string{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathSet := make(map[string]struct{})
			for _, p := range tt.paths {
				pathSet[p] = struct{}{}
			}

			result := resolveImportAliases(pathSet, &mockResolver{})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveImportAliases_VersionedPaths(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected map[string]string
	}{
		{
			name:     "chi v5 resolves to chi",
			paths:    []string{"github.com/go-chi/chi/v5"},
			expected: map[string]string{"github.com/go-chi/chi/v5": ""},
		},
		{
			name:     "yaml v3 resolves to yaml",
			paths:    []string{"gopkg.in/yaml.v3"},
			expected: map[string]string{"gopkg.in/yaml.v3": ""},
		},
		{
			name:     "multiple versioned paths with same resolved name get aliased",
			paths:    []string{"github.com/go-chi/chi/v5", "github.com/other/chi"},
			expected: map[string]string{"github.com/go-chi/chi/v5": "", "github.com/other/chi": "chi1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathSet := make(map[string]struct{})
			for _, p := range tt.paths {
				pathSet[p] = struct{}{}
			}

			result := resolveImportAliases(pathSet, &versionedPathResolver{})
			assert.Equal(t, tt.expected, result)
		})
	}
}
