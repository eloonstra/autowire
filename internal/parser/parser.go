package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/eloonstra/autowire/internal/types"
)

const (
	annotationProvide = "//autowire:provide"
	annotationInvoke  = "//autowire:invoke"
	goListOutputParts = 2
)

type fileContext struct {
	importPath string
	imports    map[string]string
}

func GetOutputInfo(outDir string) (packageName, importPath string, err error) {
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return "", "", err
	}

	importPath, err = getBasePath(absOutDir)
	if err != nil {
		return "", "", fmt.Errorf("getting module path: %w", err)
	}

	entries, err := os.ReadDir(absOutDir)
	if err != nil {
		packageName = filepath.Base(absOutDir)
		return packageName, importPath, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		hasGoSuffix := strings.HasSuffix(name, ".go")
		isTestFile := strings.HasSuffix(name, "_test.go")
		isGenFile := strings.HasSuffix(name, "_gen.go")
		if !hasGoSuffix || isTestFile || isGenFile {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(absOutDir, name), nil, parser.PackageClauseOnly)
		if err != nil {
			continue
		}
		return file.Name.Name, importPath, nil
	}

	packageName = filepath.Base(absOutDir)
	return packageName, importPath, nil
}

func Parse(scanDir string) (*types.ParseResult, error) {
	result := &types.ParseResult{}

	absDir, err := filepath.Abs(scanDir)
	if err != nil {
		return nil, err
	}

	scanBasePath, err := getBasePath(absDir)
	if err != nil {
		return nil, fmt.Errorf("getting module path: %w", err)
	}

	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if shouldSkip(d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}

		importPath := scanBasePath
		rel, err := filepath.Rel(absDir, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}
		if rel != "." {
			importPath = scanBasePath + "/" + filepath.ToSlash(rel)
		}

		return parseFile(path, importPath, result)
	})

	return result, err
}

func getBasePath(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Dir}}")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", goListOutputParts)
	if len(parts) != goListOutputParts {
		return "", fmt.Errorf("unexpected go list output: %s", out)
	}

	rel, err := filepath.Rel(parts[1], dir)
	if err != nil {
		return "", err
	}

	if rel == "." {
		return parts[0], nil
	}
	return parts[0] + "/" + filepath.ToSlash(rel), nil
}

func shouldSkip(d fs.DirEntry) bool {
	name := d.Name()
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	if d.IsDir() {
		return false // TODO: Support excluding of files and folders through flag.
	}
	return false
}

func parseFile(path, importPath string, result *types.ParseResult) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	ctx := &fileContext{
		importPath: importPath,
		imports:    buildImportMap(file),
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			hasProvide, provideArg := parseAnnotation(d.Doc, annotationProvide)
			if !hasProvide {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				p, err := parseStructProvider(ts.Name.Name, st, ctx, provideArg)
				if err != nil {
					return err
				}
				result.Providers = append(result.Providers, p)
			}

		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			hasProvide, provideArg := parseAnnotation(d.Doc, annotationProvide)
			hasInvoke, _ := parseAnnotation(d.Doc, annotationInvoke)
			if hasProvide && hasInvoke {
				return fmt.Errorf("%s: cannot have both provide and invoke annotations", d.Name.Name)
			}
			if hasProvide {
				p, err := parseFuncProvider(d, ctx, provideArg)
				if err != nil {
					return err
				}
				result.Providers = append(result.Providers, p)
			}
			if hasInvoke {
				inv, err := parseInvocation(d, ctx)
				if err != nil {
					return err
				}
				result.Invocations = append(result.Invocations, inv)
			}
		}
	}

	return nil
}

func buildImportMap(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		imports[name] = path
	}
	return imports
}

func parseAnnotation(doc *ast.CommentGroup, annotation string) (found bool, arg string) {
	if doc == nil {
		return false, ""
	}
	target := strings.TrimPrefix(annotation, "//")
	for _, c := range doc.List {
		text := strings.TrimPrefix(c.Text, "//")
		text = strings.TrimSpace(text)
		if text == target {
			return true, ""
		}
		if !strings.HasPrefix(text, target+" ") {
			continue
		}
		arg = strings.TrimSpace(strings.TrimPrefix(text, target))
		return true, arg
	}
	return false, ""
}

func resolveInterfaceFromArg(arg string, ctx *fileContext) (types.TypeRef, error) {
	parts := strings.SplitN(arg, ".", 2)
	if len(parts) == 1 {
		return types.TypeRef{Name: arg, ImportPath: ctx.importPath}, nil
	}
	pkgAlias, typeName := parts[0], parts[1]
	importPath, ok := ctx.imports[pkgAlias]
	if !ok {
		return types.TypeRef{}, fmt.Errorf("unknown package alias: %s", pkgAlias)
	}
	return types.TypeRef{Name: typeName, ImportPath: importPath}, nil
}

func parseStructProvider(name string, st *ast.StructType, ctx *fileContext, interfaceArg string) (types.Provider, error) {
	var deps []types.Dependency
	if st.Fields != nil {
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 || !isExported(field.Names[0].Name) {
				continue
			}
			t, err := resolveType(field.Type, ctx)
			if err != nil {
				return types.Provider{}, fmt.Errorf("field %s: %w", field.Names[0].Name, err)
			}
			deps = append(deps, types.Dependency{
				FieldName: field.Names[0].Name,
				Type:      t,
			})
		}
	}

	providedType := types.TypeRef{Name: name, ImportPath: ctx.importPath, IsPointer: true}
	if interfaceArg != "" {
		resolved, err := resolveInterfaceFromArg(interfaceArg, ctx)
		if err != nil {
			return types.Provider{}, fmt.Errorf("resolving interface %s: %w", interfaceArg, err)
		}
		providedType = resolved
	}

	return types.Provider{
		Name:         name,
		Kind:         types.ProviderKindStruct,
		ProvidedType: providedType,
		Dependencies: deps,
		ImportPath:   ctx.importPath,
		VarName:      toLowerCamel(name),
	}, nil
}

func parseFuncProvider(fn *ast.FuncDecl, ctx *fileContext, interfaceArg string) (types.Provider, error) {
	resultCount := 0
	if fn.Type.Results != nil {
		resultCount = len(fn.Type.Results.List)
	}

	if resultCount == 0 {
		return types.Provider{}, fmt.Errorf("%s: provider must return a value", fn.Name.Name)
	}
	if resultCount > 2 {
		return types.Provider{}, fmt.Errorf("%s: provider must return 1 or 2 values, got %d", fn.Name.Name, resultCount)
	}
	if resultCount == 2 && !isErrorType(fn.Type.Results.List[1].Type) {
		return types.Provider{}, fmt.Errorf("%s: second return value must be error", fn.Name.Name)
	}

	deps, err := parseParams(fn.Type.Params, ctx)
	if err != nil {
		return types.Provider{}, fmt.Errorf("%s: %w", fn.Name.Name, err)
	}

	provided, err := resolveType(fn.Type.Results.List[0].Type, ctx)
	if err != nil {
		return types.Provider{}, fmt.Errorf("%s return type: %w", fn.Name.Name, err)
	}

	if interfaceArg != "" {
		provided, err = resolveInterfaceFromArg(interfaceArg, ctx)
		if err != nil {
			return types.Provider{}, fmt.Errorf("%s: resolving interface %s: %w", fn.Name.Name, interfaceArg, err)
		}
	}

	canError := resultCount == 2

	return types.Provider{
		Name:         fn.Name.Name,
		Kind:         types.ProviderKindFunc,
		ProvidedType: provided,
		Dependencies: deps,
		CanError:     canError,
		ImportPath:   ctx.importPath,
		VarName:      toLowerCamel(provided.Name),
	}, nil
}

func parseInvocation(fn *ast.FuncDecl, ctx *fileContext) (types.Invocation, error) {
	params, err := parseParams(fn.Type.Params, ctx)
	if err != nil {
		return types.Invocation{}, fmt.Errorf("%s: %w", fn.Name.Name, err)
	}

	var deps []types.TypeRef
	for _, d := range params {
		deps = append(deps, d.Type)
	}

	canError := false
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		last := fn.Type.Results.List[len(fn.Type.Results.List)-1]
		canError = isErrorType(last.Type)
	}

	return types.Invocation{
		Name:         fn.Name.Name,
		Dependencies: deps,
		CanError:     canError,
		ImportPath:   ctx.importPath,
	}, nil
}

func parseParams(params *ast.FieldList, ctx *fileContext) ([]types.Dependency, error) {
	if params == nil {
		return nil, nil
	}
	var deps []types.Dependency
	for _, p := range params.List {
		t, err := resolveType(p.Type, ctx)
		if err != nil {
			return nil, err
		}
		count := len(p.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			deps = append(deps, types.Dependency{Type: t})
		}
	}
	return deps, nil
}

func resolveType(expr ast.Expr, ctx *fileContext) (types.TypeRef, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		if isBuiltin(t.Name) {
			return types.TypeRef{Name: t.Name}, nil
		}
		return types.TypeRef{Name: t.Name, ImportPath: ctx.importPath}, nil
	case *ast.StarExpr:
		inner, err := resolveType(t.X, ctx)
		if err != nil {
			return types.TypeRef{}, err
		}
		inner.IsPointer = true
		return inner, nil
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			importPath, ok := ctx.imports[pkg.Name]
			if !ok {
				return types.TypeRef{}, fmt.Errorf("unknown package alias: %s", pkg.Name)
			}
			return types.TypeRef{Name: t.Sel.Name, ImportPath: importPath}, nil
		}
	case *ast.ArrayType:
		return types.TypeRef{}, fmt.Errorf("array types not supported as dependencies")
	case *ast.MapType:
		return types.TypeRef{}, fmt.Errorf("map types not supported as dependencies")
	case *ast.ChanType:
		return types.TypeRef{}, fmt.Errorf("channel types not supported as dependencies")
	case *ast.InterfaceType:
		return types.TypeRef{}, fmt.Errorf("anonymous interface types not supported")
	case *ast.FuncType:
		return types.TypeRef{}, fmt.Errorf("function types not supported as dependencies")
	}
	return types.TypeRef{}, fmt.Errorf("unsupported type expression: %T", expr)
}

var builtins = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true,
	"float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true,
}

func isBuiltin(name string) bool  { return builtins[name] }
func isErrorType(e ast.Expr) bool { id, ok := e.(*ast.Ident); return ok && id.Name == "error" }
func isExported(name string) bool { return len(name) > 0 && unicode.IsUpper(rune(name[0])) }
func toLowerCamel(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return s
	}
	upper := 0
	for upper < n && unicode.IsUpper(runes[upper]) {
		upper++
	}
	if upper == 0 {
		return s
	}
	if upper > 1 && upper < n {
		upper--
	}
	return strings.ToLower(string(runes[:upper])) + string(runes[upper:])
}
