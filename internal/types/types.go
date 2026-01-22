package types

type PackageNameResolver interface {
	ResolveName(importPath string) string
}

type ProviderKind int

const (
	ProviderKindStruct ProviderKind = iota
	ProviderKindFunc
)

type TypeRef struct {
	Name       string
	ImportPath string
	IsPointer  bool
}

func (t TypeRef) Key() string {
	prefix := ""
	if t.IsPointer {
		prefix = "*"
	}
	if t.ImportPath == "" {
		return prefix + t.Name
	}
	return prefix + t.ImportPath + "." + t.Name
}

type Dependency struct {
	FieldName string
	Type      TypeRef
}

type Provider struct {
	Name         string
	Kind         ProviderKind
	ProvidedType TypeRef
	Dependencies []Dependency
	CanError     bool
	ImportPath   string
	VarName      string
}

type Invocation struct {
	Name         string
	Dependencies []TypeRef
	CanError     bool
	ImportPath   string
}

type ParseResult struct {
	Providers        []Provider
	Invocations      []Invocation
	OutputPackage    string
	OutputImportPath string
	OutputPath       string
}
