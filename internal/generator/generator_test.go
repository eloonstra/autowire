package generator

import (
	"bytes"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eloonstra/autowire/internal/analyzer"
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

func TestToUpper(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "foo", "Foo"},
		{"already upper", "Foo", "Foo"},
		{"empty", "", ""},
		{"single char", "a", "A"},
		{"all caps", "FOO", "FOO"},
		{"mixed case", "fooBar", "FooBar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toUpper(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPkgName(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		imports    map[string]string
		expected   string
	}{
		{
			name:       "with alias",
			importPath: "pkg/config",
			imports:    map[string]string{"pkg/config": "cfg"},
			expected:   "cfg",
		},
		{
			name:       "without alias",
			importPath: "pkg/config",
			imports:    map[string]string{"pkg/config": ""},
			expected:   "config",
		},
		{
			name:       "nested path without alias",
			importPath: "github.com/example/pkg/config",
			imports:    map[string]string{"github.com/example/pkg/config": ""},
			expected:   "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pkgName(tt.importPath, tt.imports, &mockResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPkgName_VersionedPaths(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		imports    map[string]string
		expected   string
	}{
		{
			name:       "chi v5 without alias resolves to chi",
			importPath: "github.com/go-chi/chi/v5",
			imports:    map[string]string{"github.com/go-chi/chi/v5": ""},
			expected:   "chi",
		},
		{
			name:       "chi v5 with alias uses alias",
			importPath: "github.com/go-chi/chi/v5",
			imports:    map[string]string{"github.com/go-chi/chi/v5": "router"},
			expected:   "router",
		},
		{
			name:       "yaml v3 without alias resolves to yaml",
			importPath: "gopkg.in/yaml.v3",
			imports:    map[string]string{"gopkg.in/yaml.v3": ""},
			expected:   "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pkgName(tt.importPath, tt.imports, &versionedPathResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFormatType(t *testing.T) {
	const outPath = "example.com/app"

	tests := []struct {
		name     string
		typeRef  types.TypeRef
		imports  map[string]string
		expected string
	}{
		{
			name:     "local type",
			typeRef:  types.TypeRef{Name: "Config", ImportPath: outPath},
			imports:  map[string]string{},
			expected: "Config",
		},
		{
			name:     "pointer local",
			typeRef:  types.TypeRef{Name: "Config", ImportPath: outPath, IsPointer: true},
			imports:  map[string]string{},
			expected: "*Config",
		},
		{
			name:     "external type",
			typeRef:  types.TypeRef{Name: "Config", ImportPath: "pkg/config"},
			imports:  map[string]string{"pkg/config": ""},
			expected: "config.Config",
		},
		{
			name:     "external pointer",
			typeRef:  types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
			imports:  map[string]string{"pkg/config": ""},
			expected: "*config.Config",
		},
		{
			name:     "external with alias",
			typeRef:  types.TypeRef{Name: "Config", ImportPath: "pkg/config"},
			imports:  map[string]string{"pkg/config": "cfg"},
			expected: "cfg.Config",
		},
		{
			name:     "builtin",
			typeRef:  types.TypeRef{Name: "string", ImportPath: ""},
			imports:  map[string]string{},
			expected: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatType(tt.typeRef, outPath, tt.imports, &mockResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestQualifiedName(t *testing.T) {
	const outPath = "example.com/app"

	tests := []struct {
		name       string
		funcName   string
		importPath string
		imports    map[string]string
		expected   string
	}{
		{
			name:       "same as out",
			funcName:   "NewConfig",
			importPath: outPath,
			imports:    map[string]string{},
			expected:   "NewConfig",
		},
		{
			name:       "different pkg",
			funcName:   "NewConfig",
			importPath: "pkg/config",
			imports:    map[string]string{"pkg/config": ""},
			expected:   "config.NewConfig",
		},
		{
			name:       "with alias",
			funcName:   "NewConfig",
			importPath: "pkg/config",
			imports:    map[string]string{"pkg/config": "cfg"},
			expected:   "cfg.NewConfig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qualifiedName(tt.funcName, tt.importPath, outPath, tt.imports, &mockResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMakeArgs(t *testing.T) {
	tests := []struct {
		name     string
		deps     []types.Dependency
		vars     map[string]string
		expected string
	}{
		{
			name:     "empty deps",
			deps:     []types.Dependency{},
			vars:     map[string]string{},
			expected: "",
		},
		{
			name: "single dep",
			deps: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true}},
			},
			vars:     map[string]string{"*pkg.Config": "config"},
			expected: "config",
		},
		{
			name: "multiple deps",
			deps: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: "pkg", IsPointer: true}},
				{Type: types.TypeRef{Name: "Database", ImportPath: "pkg", IsPointer: true}},
				{Type: types.TypeRef{Name: "Logger", ImportPath: "pkg", IsPointer: true}},
			},
			vars: map[string]string{
				"*pkg.Config":   "config",
				"*pkg.Database": "database",
				"*pkg.Logger":   "logger",
			},
			expected: "config, database, logger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeArgs(tt.deps, tt.vars)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestWriteImports(t *testing.T) {
	tests := []struct {
		name     string
		imports  map[string]string
		contains []string
		excludes []string
	}{
		{
			name:     "empty imports",
			imports:  map[string]string{},
			excludes: []string{"import"},
		},
		{
			name:     "single import no alias",
			imports:  map[string]string{"pkg/config": ""},
			contains: []string{"import (", `"pkg/config"`},
		},
		{
			name:     "single import with alias",
			imports:  map[string]string{"pkg/config": "cfg"},
			contains: []string{"import (", `cfg "pkg/config"`},
		},
		{
			name: "multiple imports sorted",
			imports: map[string]string{
				"pkg/zebra":  "",
				"pkg/alpha":  "",
				"pkg/middle": "mid",
			},
			contains: []string{`"pkg/alpha"`, `mid "pkg/middle"`, `"pkg/zebra"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeImports(&buf, tt.imports)
			result := buf.String()

			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
			for _, e := range tt.excludes {
				assert.NotContains(t, result, e)
			}
		})
	}
}

func TestWriteAppStruct(t *testing.T) {
	const outPath = "example.com/app"
	imports := map[string]string{"pkg/config": "", "pkg/db": ""}

	providers := []types.Provider{
		{
			VarName:      "config",
			ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
		},
		{
			VarName:      "database",
			ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg/db", IsPointer: true},
		},
	}

	var buf bytes.Buffer
	writeAppStruct(&buf, providers, outPath, imports, &mockResolver{})
	result := buf.String()

	assert.Contains(t, result, "type App struct {")
	assert.Contains(t, result, "Config *config.Config")
	assert.Contains(t, result, "Database *db.Database")
}

func TestWriteStructInit(t *testing.T) {
	const outPath = "example.com/app"

	tests := []struct {
		name     string
		provider types.Provider
		vars     map[string]string
		contains []string
	}{
		{
			name: "no dependencies",
			provider: types.Provider{
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
			},
			vars:     map[string]string{},
			contains: []string{"config := &config.Config{}"},
		},
		{
			name: "with dependencies",
			provider: types.Provider{
				VarName:      "service",
				ProvidedType: types.TypeRef{Name: "Service", ImportPath: "pkg/service", IsPointer: true},
				Dependencies: []types.Dependency{
					{FieldName: "Config", Type: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true}},
				},
			},
			vars:     map[string]string{"*pkg/config.Config": "config"},
			contains: []string{"service := &service.Service{", "Config: config,"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localImports := map[string]string{"pkg/config": "", "pkg/service": ""}
			var buf bytes.Buffer
			writeStructInit(&buf, tt.provider, tt.vars, outPath, localImports, &mockResolver{})
			result := buf.String()

			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
		})
	}
}

func TestWriteFuncInit(t *testing.T) {
	const outPath = "example.com/app"

	tests := []struct {
		name     string
		provider types.Provider
		vars     map[string]string
		contains []string
		excludes []string
	}{
		{
			name: "no error",
			provider: types.Provider{
				Name:         "NewConfig",
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				CanError:     false,
			},
			vars:     map[string]string{},
			contains: []string{"config := config.NewConfig()"},
			excludes: []string{"err :=", "if err != nil"},
		},
		{
			name: "with error",
			provider: types.Provider{
				Name:         "NewDatabase",
				VarName:      "database",
				ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg/db", IsPointer: true},
				ImportPath:   "pkg/db",
				CanError:     true,
				Dependencies: []types.Dependency{
					{Type: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true}},
				},
			},
			vars:     map[string]string{"*pkg/config.Config": "config"},
			contains: []string{"database, err := db.NewDatabase(config)", "if err != nil {", "return nil, err"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localImports := map[string]string{"pkg/config": "", "pkg/db": ""}
			var buf bytes.Buffer
			writeFuncInit(&buf, tt.provider, tt.vars, outPath, localImports, &mockResolver{})
			result := buf.String()

			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
			for _, e := range tt.excludes {
				assert.NotContains(t, result, e)
			}
		})
	}
}

func TestWriteInvocation(t *testing.T) {
	const outPath = "example.com/app"
	imports := map[string]string{"pkg/setup": ""}

	tests := []struct {
		name       string
		invocation types.Invocation
		vars       map[string]string
		contains   []string
		excludes   []string
	}{
		{
			name: "no error",
			invocation: types.Invocation{
				Name:       "Setup",
				ImportPath: "pkg/setup",
				CanError:   false,
			},
			vars:     map[string]string{},
			contains: []string{"setup.Setup()"},
			excludes: []string{"if err :="},
		},
		{
			name: "with error",
			invocation: types.Invocation{
				Name:       "SetupRoutes",
				ImportPath: "pkg/setup",
				CanError:   true,
				Dependencies: []types.TypeRef{
					{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				},
			},
			vars:     map[string]string{"*pkg/config.Config": "config"},
			contains: []string{"if err := setup.SetupRoutes(config); err != nil {", "return nil, err"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeInvocation(&buf, tt.invocation, tt.vars, outPath, imports, &mockResolver{})
			result := buf.String()

			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
			for _, e := range tt.excludes {
				assert.NotContains(t, result, e)
			}
		})
	}
}

func TestGenerate_EmptyResult(t *testing.T) {
	result := &analyzer.Result{
		Providers:        []types.Provider{},
		Invocations:      []types.Invocation{},
		PackageName:      "main",
		OutputImportPath: "example.com/app",
		Imports:          map[string]string{},
	}

	output, err := Generate(result, &mockResolver{})
	require.NoError(t, err)

	outputStr := string(output)
	assert.Contains(t, outputStr, "package main")
	assert.Contains(t, outputStr, "type App struct {")
	assert.Contains(t, outputStr, "func InitializeApp() (*App, error)")

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "", output, parser.AllErrors)
	assert.NoError(t, err, "generated code should be valid Go")
}

func TestGenerate_SingleProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider types.Provider
		imports  map[string]string
		contains []string
	}{
		{
			name: "struct provider no deps",
			provider: types.Provider{
				Name:         "Config",
				Kind:         types.ProviderKindStruct,
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
			},
			imports:  map[string]string{"pkg/config": ""},
			contains: []string{"config := &config.Config{}"},
		},
		{
			name: "func provider no error",
			provider: types.Provider{
				Name:         "NewConfig",
				Kind:         types.ProviderKindFunc,
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				CanError:     false,
			},
			imports:  map[string]string{"pkg/config": ""},
			contains: []string{"config := config.NewConfig()"},
		},
		{
			name: "func provider with error",
			provider: types.Provider{
				Name:         "NewConfig",
				Kind:         types.ProviderKindFunc,
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
				CanError:     true,
			},
			imports:  map[string]string{"pkg/config": ""},
			contains: []string{"config, err := config.NewConfig()", "if err != nil {"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &analyzer.Result{
				Providers:        []types.Provider{tt.provider},
				Invocations:      []types.Invocation{},
				PackageName:      "main",
				OutputImportPath: "example.com/app",
				Imports:          tt.imports,
			}

			output, err := Generate(result, &mockResolver{})
			require.NoError(t, err)

			outputStr := string(output)
			for _, c := range tt.contains {
				assert.Contains(t, outputStr, c)
			}

			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "", output, parser.AllErrors)
			assert.NoError(t, err, "generated code should be valid Go")
		})
	}
}

func TestGenerate_WithInvocations(t *testing.T) {
	result := &analyzer.Result{
		Providers: []types.Provider{
			{
				Name:         "NewConfig",
				Kind:         types.ProviderKindFunc,
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
			},
		},
		Invocations: []types.Invocation{
			{
				Name:       "Setup",
				ImportPath: "pkg/setup",
				CanError:   true,
				Dependencies: []types.TypeRef{
					{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				},
			},
		},
		PackageName:      "main",
		OutputImportPath: "example.com/app",
		Imports:          map[string]string{"pkg/config": "", "pkg/setup": ""},
	}

	output, err := Generate(result, &mockResolver{})
	require.NoError(t, err)

	outputStr := string(output)
	assert.Contains(t, outputStr, "// provide")
	assert.Contains(t, outputStr, "// invoke")
	assert.Contains(t, outputStr, "setup.Setup(config)")

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "", output, parser.AllErrors)
	assert.NoError(t, err, "generated code should be valid Go")
}

func TestGenerate_FullOutput(t *testing.T) {
	result := &analyzer.Result{
		Providers: []types.Provider{
			{
				Name:         "NewConfig",
				Kind:         types.ProviderKindFunc,
				VarName:      "config",
				ProvidedType: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true},
				ImportPath:   "pkg/config",
			},
			{
				Name:         "NewDatabase",
				Kind:         types.ProviderKindFunc,
				VarName:      "database",
				ProvidedType: types.TypeRef{Name: "Database", ImportPath: "pkg/db", IsPointer: true},
				ImportPath:   "pkg/db",
				CanError:     true,
				Dependencies: []types.Dependency{
					{Type: types.TypeRef{Name: "Config", ImportPath: "pkg/config", IsPointer: true}},
				},
			},
			{
				Name:         "Service",
				Kind:         types.ProviderKindStruct,
				VarName:      "service",
				ProvidedType: types.TypeRef{Name: "Service", ImportPath: "pkg/service", IsPointer: true},
				ImportPath:   "pkg/service",
				Dependencies: []types.Dependency{
					{FieldName: "DB", Type: types.TypeRef{Name: "Database", ImportPath: "pkg/db", IsPointer: true}},
				},
			},
		},
		Invocations: []types.Invocation{
			{
				Name:       "SetupRoutes",
				ImportPath: "pkg/routes",
				CanError:   true,
				Dependencies: []types.TypeRef{
					{Name: "Service", ImportPath: "pkg/service", IsPointer: true},
				},
			},
		},
		PackageName:      "main",
		OutputImportPath: "example.com/app",
		Imports: map[string]string{
			"pkg/config":  "",
			"pkg/db":      "",
			"pkg/service": "",
			"pkg/routes":  "",
		},
	}

	output, err := Generate(result, &mockResolver{})
	require.NoError(t, err)

	outputStr := string(output)

	assert.Contains(t, outputStr, "// Code generated by autowire. DO NOT EDIT.")
	assert.Contains(t, outputStr, "package main")
	assert.Contains(t, outputStr, "type App struct {")
	assert.Contains(t, outputStr, "*config.Config")
	assert.Contains(t, outputStr, "*db.Database")
	assert.Contains(t, outputStr, "*service.Service")
	assert.Contains(t, outputStr, "func InitializeApp() (*App, error)")
	assert.Contains(t, outputStr, "config := config.NewConfig()")
	assert.Contains(t, outputStr, "database, err := db.NewDatabase(config)")
	assert.Contains(t, outputStr, "service := &service.Service{")
	assert.Contains(t, outputStr, "DB: database,")
	assert.Contains(t, outputStr, "routes.SetupRoutes(service)")

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "", output, parser.AllErrors)
	assert.NoError(t, err, "generated code should be valid Go")

	lines := strings.Split(outputStr, "\n")
	var configLine, dbLine, serviceLine int
	for i, line := range lines {
		if strings.Contains(line, "config := config.NewConfig()") {
			configLine = i
		}
		if strings.Contains(line, "database, err := db.NewDatabase") {
			dbLine = i
		}
		if strings.Contains(line, "service := &service.Service{") {
			serviceLine = i
		}
	}
	assert.Less(t, configLine, dbLine, "config should be initialized before database")
	assert.Less(t, dbLine, serviceLine, "database should be initialized before service")
}
