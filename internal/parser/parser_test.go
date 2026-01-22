package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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
		"github.com/go-chi/chi/v4": "chi",
		"gopkg.in/yaml.v3":         "yaml",
	}
	if name, ok := knownPackages[importPath]; ok {
		return name
	}
	return filepath.Base(importPath)
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return m.isDir }
func (m mockDirEntry) Type() fs.FileMode          { return 0 }
func (m mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		isDir    bool
		expected bool
	}{
		{"hidden file", ".hidden", false, true},
		{"hidden dir", ".git", true, true},
		{"underscore file", "_ignore.go", false, true},
		{"underscore dir", "_build", true, true},
		{"normal file", "main.go", false, false},
		{"normal dir", "pkg", true, false},
		{"double underscore", "__test__", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := mockDirEntry{name: tt.fileName, isDir: tt.isDir}
			got := shouldSkip(entry)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		comments   []string
		annotation string
		wantFound  bool
		wantArg    string
	}{
		{
			name:       "exact match provide",
			comments:   []string{"//autowire:provide"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "",
		},
		{
			name:       "with space",
			comments:   []string{"// autowire:provide"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "",
		},
		{
			name:       "with local interface arg",
			comments:   []string{"//autowire:provide Reader"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "Reader",
		},
		{
			name:       "with package interface arg",
			comments:   []string{"//autowire:provide io.Reader"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "io.Reader",
		},
		{
			name:       "with space before arg",
			comments:   []string{"// autowire:provide  Reader"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "Reader",
		},
		{
			name:       "wrong annotation",
			comments:   []string{"//autowire:invoke"},
			annotation: annotationProvide,
			wantFound:  false,
			wantArg:    "",
		},
		{
			name:       "invoke annotation",
			comments:   []string{"//autowire:invoke"},
			annotation: annotationInvoke,
			wantFound:  true,
			wantArg:    "",
		},
		{
			name:       "multiple comments with provide",
			comments:   []string{"// Some comment", "//autowire:provide"},
			annotation: annotationProvide,
			wantFound:  true,
			wantArg:    "",
		},
		{
			name:       "empty comments",
			comments:   []string{},
			annotation: annotationProvide,
			wantFound:  false,
			wantArg:    "",
		},
		{
			name:       "unrelated comment",
			comments:   []string{"// This is a comment"},
			annotation: annotationProvide,
			wantFound:  false,
			wantArg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc *ast.CommentGroup
			if len(tt.comments) > 0 {
				var list []*ast.Comment
				for _, c := range tt.comments {
					list = append(list, &ast.Comment{Text: c})
				}
				doc = &ast.CommentGroup{List: list}
			}
			found, arg := parseAnnotation(doc, tt.annotation)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantArg, arg)
		})
	}

	t.Run("nil doc", func(t *testing.T) {
		found, arg := parseAnnotation(nil, annotationProvide)
		assert.False(t, found)
		assert.Empty(t, arg)
	})
}

func TestIsBuiltin(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		expected bool
	}{
		{"string", "string", true},
		{"int", "int", true},
		{"int8", "int8", true},
		{"int16", "int16", true},
		{"int32", "int32", true},
		{"int64", "int64", true},
		{"uint", "uint", true},
		{"uint8", "uint8", true},
		{"uint16", "uint16", true},
		{"uint32", "uint32", true},
		{"uint64", "uint64", true},
		{"uintptr", "uintptr", true},
		{"float32", "float32", true},
		{"float64", "float64", true},
		{"complex64", "complex64", true},
		{"complex128", "complex128", true},
		{"bool", "bool", true},
		{"byte", "byte", true},
		{"rune", "rune", true},
		{"error", "error", true},
		{"any", "any", true},
		{"comparable", "comparable", true},
		{"custom type", "MyType", false},
		{"config", "Config", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBuiltin(tt.typeName)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"uppercase start", "Foo", true},
		{"lowercase start", "foo", false},
		{"single uppercase", "A", true},
		{"single lowercase", "a", false},
		{"empty string", "", false},
		{"underscore start", "_foo", false},
		{"number start", "123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExported(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestToLowerCamel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "UserService", "userService"},
		{"all caps prefix", "HTTPClient", "httpClient"},
		{"single char", "A", "a"},
		{"already lower", "user", "user"},
		{"empty", "", ""},
		{"all uppercase short", "ID", "id"},
		{"all uppercase long", "HTTP", "http"},
		{"mixed", "APIService", "apiService"},
		{"single uppercase in middle", "userName", "userName"},
		{"URL prefix", "URLParser", "urlParser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLowerCamel(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildImportMap(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected map[string]string
	}{
		{
			name: "simple import",
			src: `package test
import "fmt"`,
			expected: map[string]string{"fmt": "fmt"},
		},
		{
			name: "aliased import",
			src: `package test
import f "fmt"`,
			expected: map[string]string{"f": "fmt"},
		},
		{
			name: "multiple imports",
			src: `package test
import (
	"fmt"
	"os"
	alias "path/filepath"
)`,
			expected: map[string]string{
				"fmt":   "fmt",
				"os":    "os",
				"alias": "path/filepath",
			},
		},
		{
			name:     "no imports",
			src:      `package test`,
			expected: map[string]string{},
		},
		{
			name: "nested path",
			src: `package test
import "github.com/example/pkg/config"`,
			expected: map[string]string{"config": "github.com/example/pkg/config"},
		},
		{
			name: "blank import filtered",
			src: `package test
import (
	"fmt"
	_ "database/sql"
)`,
			expected: map[string]string{"fmt": "fmt"},
		},
		{
			name: "dot import filtered",
			src: `package test
import (
	"fmt"
	. "strings"
)`,
			expected: map[string]string{"fmt": "fmt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ImportsOnly)
			require.NoError(t, err)

			got := buildImportMap(file, &mockResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildImportMap_VersionedPaths(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected map[string]string
	}{
		{
			name: "chi v5 versioned import",
			src: `package test
import "github.com/go-chi/chi/v5"`,
			expected: map[string]string{"chi": "github.com/go-chi/chi/v5"},
		},
		{
			name: "chi v5 with alias overrides resolver",
			src: `package test
import router "github.com/go-chi/chi/v5"`,
			expected: map[string]string{"router": "github.com/go-chi/chi/v5"},
		},
		{
			name: "yaml v3 versioned import",
			src: `package test
import "gopkg.in/yaml.v3"`,
			expected: map[string]string{"yaml": "gopkg.in/yaml.v3"},
		},
		{
			name: "multiple versioned imports",
			src: `package test
import (
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)`,
			expected: map[string]string{
				"chi":  "github.com/go-chi/chi/v5",
				"yaml": "gopkg.in/yaml.v3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ImportsOnly)
			require.NoError(t, err)

			got := buildImportMap(file, &versionedPathResolver{})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestResolveType(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name     string
		src      string
		expected types.TypeRef
		wantErr  bool
		errMsg   string
	}{
		{
			name: "ident local type",
			src: `package test
var x Foo`,
			expected: types.TypeRef{Name: "Foo", ImportPath: testImportPath},
		},
		{
			name: "ident builtin",
			src: `package test
var x string`,
			expected: types.TypeRef{Name: "string", ImportPath: ""},
		},
		{
			name: "pointer local",
			src: `package test
var x *Foo`,
			expected: types.TypeRef{Name: "Foo", ImportPath: testImportPath, IsPointer: true},
		},
		{
			name: "selector",
			src: `package test
import "pkg/bar"
var x bar.Foo`,
			expected: types.TypeRef{Name: "Foo", ImportPath: "pkg/bar"},
		},
		{
			name: "pointer selector",
			src: `package test
import "pkg/bar"
var x *bar.Foo`,
			expected: types.TypeRef{Name: "Foo", ImportPath: "pkg/bar", IsPointer: true},
		},
		{
			name: "array type error",
			src: `package test
var x []Foo`,
			wantErr: true,
			errMsg:  "array types not supported",
		},
		{
			name: "map type error",
			src: `package test
var x map[string]Foo`,
			wantErr: true,
			errMsg:  "map types not supported",
		},
		{
			name: "chan type error",
			src: `package test
var x chan Foo`,
			wantErr: true,
			errMsg:  "channel types not supported",
		},
		{
			name: "interface type error",
			src: `package test
var x interface{}`,
			wantErr: true,
			errMsg:  "anonymous interface types not supported",
		},
		{
			name: "func type error",
			src: `package test
var x func()`,
			wantErr: true,
			errMsg:  "function types not supported",
		},
		{
			name: "unknown package alias",
			src: `package test
var x unknown.Foo`,
			wantErr: true,
			errMsg:  "unknown package alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var varType ast.Expr
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							varType = valueSpec.Type
							break
						}
					}
				}
			}
			require.NotNil(t, varType, "could not find var declaration")

			got, err := resolveType(varType, ctx)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseParams(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name     string
		src      string
		expected []types.Dependency
	}{
		{
			name: "no params",
			src: `package test
func foo() {}`,
			expected: nil,
		},
		{
			name: "single param",
			src: `package test
func foo(cfg *Config) {}`,
			expected: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: testImportPath, IsPointer: true}},
			},
		},
		{
			name: "multiple params same type",
			src: `package test
func foo(a, b *Config) {}`,
			expected: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: testImportPath, IsPointer: true}},
				{Type: types.TypeRef{Name: "Config", ImportPath: testImportPath, IsPointer: true}},
			},
		},
		{
			name: "multiple params different types",
			src: `package test
func foo(cfg *Config, db *Database) {}`,
			expected: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: testImportPath, IsPointer: true}},
				{Type: types.TypeRef{Name: "Database", ImportPath: testImportPath, IsPointer: true}},
			},
		},
		{
			name: "unnamed param",
			src: `package test
func foo(*Config) {}`,
			expected: []types.Dependency{
				{Type: types.TypeRef{Name: "Config", ImportPath: testImportPath, IsPointer: true}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var params *ast.FieldList
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok {
					params = funcDecl.Type.Params
					break
				}
			}

			got, err := parseParams(params, ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseStructProvider(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name        string
		src         string
		structName  string
		expectedLen int
		checkDeps   func(t *testing.T, deps []types.Dependency)
	}{
		{
			name: "struct with no fields",
			src: `package test
type SimpleStruct struct{}`,
			structName:  "SimpleStruct",
			expectedLen: 0,
		},
		{
			name: "struct with exported fields",
			src: `package test
type StructWithDeps struct {
	Config   *Config
	Database *Database
}`,
			structName:  "StructWithDeps",
			expectedLen: 2,
			checkDeps: func(t *testing.T, deps []types.Dependency) {
				assert.Equal(t, "Config", deps[0].FieldName)
				assert.Equal(t, "Database", deps[1].FieldName)
			},
		},
		{
			name: "struct with unexported fields",
			src: `package test
type StructUnexported struct {
	Exported   *Config
	unexported *Database
}`,
			structName:  "StructUnexported",
			expectedLen: 1,
			checkDeps: func(t *testing.T, deps []types.Dependency) {
				assert.Equal(t, "Exported", deps[0].FieldName)
			},
		},
		{
			name: "struct with embedded field",
			src: `package test
type StructEmbedded struct {
	Config
	Name *Database
}`,
			structName:  "StructEmbedded",
			expectedLen: 1,
			checkDeps: func(t *testing.T, deps []types.Dependency) {
				assert.Equal(t, "Name", deps[0].FieldName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var st *ast.StructType
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if typeSpec.Name.Name == tt.structName {
								st = typeSpec.Type.(*ast.StructType)
								break
							}
						}
					}
				}
			}
			require.NotNil(t, st)

			provider, err := parseStructProvider(tt.structName, st, ctx, "")
			assert.NoError(t, err)
			assert.Equal(t, tt.structName, provider.Name)
			assert.Equal(t, types.ProviderKindStruct, provider.Kind)
			assert.True(t, provider.ProvidedType.IsPointer)
			assert.Len(t, provider.Dependencies, tt.expectedLen)

			if tt.checkDeps != nil {
				tt.checkDeps(t, provider.Dependencies)
			}
		})
	}
}

func TestParseStructProvider_WithInterface(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name         string
		src          string
		structName   string
		interfaceArg string
		expectedType types.TypeRef
	}{
		{
			name: "no interface arg returns pointer to struct",
			src: `package test
type FileReader struct{}`,
			structName:   "FileReader",
			interfaceArg: "",
			expectedType: types.TypeRef{
				Name:       "FileReader",
				ImportPath: testImportPath,
				IsPointer:  true,
			},
		},
		{
			name: "local interface",
			src: `package test
type FileReader struct{}`,
			structName:   "FileReader",
			interfaceArg: "Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: testImportPath,
				IsPointer:  false,
			},
		},
		{
			name: "imported interface",
			src: `package test
import "io"
type FileReader struct{}`,
			structName:   "FileReader",
			interfaceArg: "io.Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
				IsPointer:  false,
			},
		},
		{
			name: "aliased import",
			src: `package test
import waffle "io"
type FileReader struct{}`,
			structName:   "FileReader",
			interfaceArg: "waffle.Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
				IsPointer:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var st *ast.StructType
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == tt.structName {
							st = typeSpec.Type.(*ast.StructType)
							break
						}
					}
				}
			}
			require.NotNil(t, st)

			provider, err := parseStructProvider(tt.structName, st, ctx, tt.interfaceArg)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedType, provider.ProvidedType)
		})
	}
}

func TestParseFuncProvider(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name        string
		src         string
		funcName    string
		expectedErr string
		checkResult func(t *testing.T, p types.Provider)
	}{
		{
			name: "simple provider",
			src: `package test
func NewSimple() *Simple { return nil }`,
			funcName: "NewSimple",
			checkResult: func(t *testing.T, p types.Provider) {
				assert.Equal(t, "NewSimple", p.Name)
				assert.Equal(t, types.ProviderKindFunc, p.Kind)
				assert.False(t, p.CanError)
				assert.Len(t, p.Dependencies, 0)
			},
		},
		{
			name: "provider with deps",
			src: `package test
func NewService(cfg *Config, db *Database) *Service { return nil }`,
			funcName: "NewService",
			checkResult: func(t *testing.T, p types.Provider) {
				assert.Equal(t, "NewService", p.Name)
				assert.Len(t, p.Dependencies, 2)
				assert.False(t, p.CanError)
			},
		},
		{
			name: "provider with error",
			src: `package test
func NewWithError(cfg *Config) (*WithError, error) { return nil, nil }`,
			funcName: "NewWithError",
			checkResult: func(t *testing.T, p types.Provider) {
				assert.Equal(t, "NewWithError", p.Name)
				assert.True(t, p.CanError)
				assert.Len(t, p.Dependencies, 1)
			},
		},
		{
			name: "no return error",
			src: `package test
func NoReturn() {}`,
			funcName:    "NoReturn",
			expectedErr: "must return a value",
		},
		{
			name: "three returns error",
			src: `package test
func ThreeReturns() (*A, *B, error) { return nil, nil, nil }`,
			funcName:    "ThreeReturns",
			expectedErr: "must return 1 or 2 values",
		},
		{
			name: "wrong second return",
			src: `package test
func WrongSecond() (*Config, string) { return nil, "" }`,
			funcName:    "WrongSecond",
			expectedErr: "second return value must be error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == tt.funcName {
					fn = funcDecl
					break
				}
			}
			require.NotNil(t, fn)

			provider, err := parseFuncProvider(fn, ctx, "")

			if tt.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			assert.NoError(t, err)
			if tt.checkResult != nil {
				tt.checkResult(t, provider)
			}
		})
	}
}

func TestParseFuncProvider_WithInterface(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name         string
		src          string
		funcName     string
		interfaceArg string
		expectedType types.TypeRef
		wantErr      bool
		errMsg       string
	}{
		{
			name: "no interface arg returns concrete",
			src: `package test
func NewReader() *FileReader { return nil }`,
			funcName:     "NewReader",
			interfaceArg: "",
			expectedType: types.TypeRef{
				Name:       "FileReader",
				ImportPath: testImportPath,
				IsPointer:  true,
			},
		},
		{
			name: "local interface",
			src: `package test
func NewReader() *FileReader { return nil }`,
			funcName:     "NewReader",
			interfaceArg: "Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: testImportPath,
				IsPointer:  false,
			},
		},
		{
			name: "imported interface",
			src: `package test
import "io"
func NewReader() *FileReader { return nil }`,
			funcName:     "NewReader",
			interfaceArg: "io.Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
				IsPointer:  false,
			},
		},
		{
			name: "aliased import",
			src: `package test
import waffle "io"
func NewReader() *FileReader { return nil }`,
			funcName:     "NewReader",
			interfaceArg: "waffle.Reader",
			expectedType: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
				IsPointer:  false,
			},
		},
		{
			name: "unknown package error",
			src: `package test
func NewReader() *FileReader { return nil }`,
			funcName:     "NewReader",
			interfaceArg: "unknown.Reader",
			wantErr:      true,
			errMsg:       "unknown package alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == tt.funcName {
					fn = funcDecl
					break
				}
			}
			require.NotNil(t, fn)

			provider, err := parseFuncProvider(fn, ctx, tt.interfaceArg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedType, provider.ProvidedType)
		})
	}
}

func TestResolveInterfaceFromArg(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name     string
		arg      string
		imports  map[string]string
		expected types.TypeRef
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "local interface",
			arg:     "Reader",
			imports: map[string]string{},
			expected: types.TypeRef{
				Name:       "Reader",
				ImportPath: testImportPath,
			},
		},
		{
			name:    "imported interface io.Reader",
			arg:     "io.Reader",
			imports: map[string]string{"io": "io"},
			expected: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
			},
		},
		{
			name:    "imported interface with long path",
			arg:     "http.Handler",
			imports: map[string]string{"http": "net/http"},
			expected: types.TypeRef{
				Name:       "Handler",
				ImportPath: "net/http",
			},
		},
		{
			name:    "aliased import",
			arg:     "waffle.Reader",
			imports: map[string]string{"waffle": "io"},
			expected: types.TypeRef{
				Name:       "Reader",
				ImportPath: "io",
			},
		},
		{
			name:    "unknown package",
			arg:     "unknown.Type",
			imports: map[string]string{},
			wantErr: true,
			errMsg:  "unknown package alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fileContext{
				importPath: testImportPath,
				imports:    tt.imports,
			}
			got, err := resolveInterfaceFromArg(tt.arg, ctx)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseInvocation(t *testing.T) {
	const testImportPath = "example.com/test"

	tests := []struct {
		name        string
		src         string
		funcName    string
		checkResult func(t *testing.T, inv types.Invocation)
	}{
		{
			name: "simple invocation",
			src: `package test
func SetupSimple() {}`,
			funcName: "SetupSimple",
			checkResult: func(t *testing.T, inv types.Invocation) {
				assert.Equal(t, "SetupSimple", inv.Name)
				assert.False(t, inv.CanError)
				assert.Len(t, inv.Dependencies, 0)
			},
		},
		{
			name: "invocation with error",
			src: `package test
func SetupWithError(cfg *Config) error { return nil }`,
			funcName: "SetupWithError",
			checkResult: func(t *testing.T, inv types.Invocation) {
				assert.Equal(t, "SetupWithError", inv.Name)
				assert.True(t, inv.CanError)
				assert.Len(t, inv.Dependencies, 1)
			},
		},
		{
			name: "invocation with deps",
			src: `package test
func SetupWithDeps(cfg *Config, db *Database) {}`,
			funcName: "SetupWithDeps",
			checkResult: func(t *testing.T, inv types.Invocation) {
				assert.Equal(t, "SetupWithDeps", inv.Name)
				assert.False(t, inv.CanError)
				assert.Len(t, inv.Dependencies, 2)
			},
		},
		{
			name: "invocation returning non-error",
			src: `package test
func SetupReturnsValue() int { return 0 }`,
			funcName: "SetupReturnsValue",
			checkResult: func(t *testing.T, inv types.Invocation) {
				assert.False(t, inv.CanError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, parser.ParseComments)
			require.NoError(t, err)

			ctx := &fileContext{
				importPath: testImportPath,
				imports:    buildImportMap(file, &mockResolver{}),
			}

			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == tt.funcName {
					fn = funcDecl
					break
				}
			}
			require.NotNil(t, fn)

			inv, err := parseInvocation(fn, ctx)
			assert.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, inv)
			}
		})
	}
}

func TestParseFile_BothAnnotations(t *testing.T) {
	src := `package test

//autowire:provide
//autowire:invoke
func BothAnnotations() *Config { return nil }

type Config struct{}
`
	tmpFile, err := os.CreateTemp("", "both_annotations_*.go")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(src)
	require.NoError(t, err)
	tmpFile.Close()

	result := &types.ParseResult{}
	err = parseFile(tmpFile.Name(), "example.com/test", &mockResolver{}, result)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot have both provide and invoke")
}

func TestIsErrorType(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected bool
	}{
		{
			name:     "error type",
			src:      `package test; var x error`,
			expected: true,
		},
		{
			name:     "string type",
			src:      `package test; var x string`,
			expected: false,
		},
		{
			name:     "custom type",
			src:      `package test; var x MyError`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, 0)
			require.NoError(t, err)

			var varType ast.Expr
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							varType = valueSpec.Type
							break
						}
					}
				}
			}
			require.NotNil(t, varType)

			got := isErrorType(varType)
			assert.Equal(t, tt.expected, got)
		})
	}
}
