package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eloonstra/autowire/internal/types"
)

type Result struct {
	Providers        []types.Provider
	Invocations      []types.Invocation
	PackageName      string
	OutputImportPath string
	Imports          map[string]string
}

func Analyze(parsed *types.ParseResult, resolver types.PackageNameResolver) (*Result, error) {
	byType := make(map[string]types.Provider)
	for _, p := range parsed.Providers {
		key := p.ProvidedType.Key()
		if dup, ok := byType[key]; ok {
			return nil, fmt.Errorf("duplicate provider for %s: %s and %s", key, dup.Name, p.Name)
		}
		byType[key] = p
	}

	if err := validateDeps(parsed.Providers, parsed.Invocations, byType); err != nil {
		return nil, err
	}

	ordered, err := topoSort(parsed.Providers, parsed.Invocations, byType)
	if err != nil {
		return nil, err
	}

	resolveVarNames(ordered)

	return &Result{
		Providers:        ordered,
		Invocations:      parsed.Invocations,
		PackageName:      parsed.OutputPackage,
		OutputImportPath: parsed.OutputImportPath,
		Imports:          collectImports(ordered, parsed.Invocations, parsed.OutputImportPath, resolver),
	}, nil
}

func validateDeps(providers []types.Provider, invocations []types.Invocation, byType map[string]types.Provider) error {
	var missing []string

	for _, p := range providers {
		for _, dep := range p.Dependencies {
			if _, ok := byType[dep.Type.Key()]; !ok {
				missing = append(missing, fmt.Sprintf("%s requires %s", p.Name, dep.Type.Key()))
			}
		}
	}

	for _, inv := range invocations {
		for _, dep := range inv.Dependencies {
			if _, ok := byType[dep.Key()]; !ok {
				missing = append(missing, fmt.Sprintf("%s requires %s", inv.Name, dep.Key()))
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing dependencies:\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}

func resolveVarNames(providers []types.Provider) {
	usedNames := make(map[string]int)

	for i := range providers {
		baseName := providers[i].VarName
		count := usedNames[baseName]
		usedNames[baseName] = count + 1

		if count == 0 {
			continue
		}
		providers[i].VarName = fmt.Sprintf("%s%d", baseName, count)
	}
}

func topoSort(providers []types.Provider, invocations []types.Invocation, byType map[string]types.Provider) ([]types.Provider, error) {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var result []types.Provider

	var visit func(p types.Provider, path []string) error
	visit = func(p types.Provider, path []string) error {
		key := p.ProvidedType.Key()

		if inStack[key] {
			return fmt.Errorf("circular dependency: %s", strings.Join(append(path, key), " -> "))
		}
		if visited[key] {
			return nil
		}

		inStack[key] = true
		path = append(path, key)

		for _, dep := range p.Dependencies {
			if depProvider, ok := byType[dep.Type.Key()]; ok {
				if err := visit(depProvider, path); err != nil {
					return err
				}
			}
		}

		inStack[key] = false
		visited[key] = true
		result = append(result, p)
		return nil
	}

	for _, inv := range invocations {
		for _, dep := range inv.Dependencies {
			if p, ok := byType[dep.Key()]; ok {
				if err := visit(p, nil); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, p := range providers {
		if err := visit(p, nil); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func collectImports(providers []types.Provider, invocations []types.Invocation, outputPath string, resolver types.PackageNameResolver) map[string]string {
	paths := make(map[string]struct{})

	add := func(path string) {
		if path == "" || path == outputPath {
			return
		}
		paths[path] = struct{}{}
	}

	for _, p := range providers {
		add(p.ImportPath)
		for _, dep := range p.Dependencies {
			add(dep.Type.ImportPath)
		}
	}

	for _, inv := range invocations {
		add(inv.ImportPath)
		for _, dep := range inv.Dependencies {
			add(dep.ImportPath)
		}
	}

	return resolveImportAliases(paths, resolver)
}

func resolveImportAliases(paths map[string]struct{}, resolver types.PackageNameResolver) map[string]string {
	imports := make(map[string]string)
	baseCount := make(map[string]int)

	sortedPaths := make([]string, 0, len(paths))
	for p := range paths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		base := resolver.ResolveName(path)
		count := baseCount[base]
		baseCount[base] = count + 1

		if count == 0 {
			imports[path] = ""
			continue
		}
		imports[path] = fmt.Sprintf("%s%d", base, count)
	}
	return imports
}
