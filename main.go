package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eloonstra/autowire/internal/analyzer"
	"github.com/eloonstra/autowire/internal/generator"
	"github.com/eloonstra/autowire/internal/parser"
	"github.com/eloonstra/autowire/internal/types"
	"github.com/spf13/cobra"
)

const (
	defaultOutputFileName = "app_gen.go"
	filePermission        = 0644
)

var (
	scanDirs   []string
	outDir     string
	outputName string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "autowire",
	Short: "Autowire generates dependency injection code from annotations",
	Long: `Autowire scans Go source files for autowire annotations and generates
dependency injection wiring code automatically.

It parses provider and invocation annotations, analyzes dependencies,
and generates a single output file containing all the wiring code.`,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringArrayVarP(&scanDirs, "scan", "s", []string{"."}, "directories to scan for autowire annotations (can be specified multiple times)")
	rootCmd.Flags().StringVarP(&outDir, "out", "o", ".", "output directory for generated code")
	rootCmd.Flags().StringVarP(&outputName, "name", "n", defaultOutputFileName, "output filename")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(*cobra.Command, []string) error {
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("resolving output directory: %w", err)
	}

	if verbose {
		fmt.Printf("output dir: %s\n", absOutDir)
	}

	outputPackage, outputImportPath, err := parser.GetOutputInfo(absOutDir)
	if err != nil {
		return fmt.Errorf("getting output info: %w", err)
	}

	merged := &types.ParseResult{
		OutputPath:       absOutDir,
		OutputPackage:    outputPackage,
		OutputImportPath: outputImportPath,
	}

	for _, dir := range scanDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolving directory %s: %w", dir, err)
		}

		if verbose {
			fmt.Printf("scanning: %s\n", absDir)
		}

		parsed, err := parser.Parse(absDir)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", dir, err)
		}

		merged.Providers = append(merged.Providers, parsed.Providers...)
		merged.Invocations = append(merged.Invocations, parsed.Invocations...)
	}

	if len(merged.Providers) == 0 && len(merged.Invocations) == 0 {
		return fmt.Errorf("no autowire annotations found in: %s", strings.Join(scanDirs, ", "))
	}

	if verbose {
		fmt.Printf("found %d providers:\n", len(merged.Providers))
		for _, p := range merged.Providers {
			fmt.Printf("  - %s -> %s\n", p.Name, p.ProvidedType.Key())
		}
		fmt.Printf("found %d invocations:\n", len(merged.Invocations))
		for _, inv := range merged.Invocations {
			fmt.Printf("  - %s\n", inv.Name)
		}
	}

	result, err := analyzer.Analyze(merged)
	if err != nil {
		return fmt.Errorf("analyzing: %w", err)
	}

	if verbose {
		fmt.Printf("initialization order:\n")
		for i, p := range result.Providers {
			fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.VarName)
		}
	}

	code, err := generator.Generate(result)
	if err != nil {
		return fmt.Errorf("generating: %w", err)
	}

	outputPath := filepath.Join(absOutDir, outputName)
	if err := os.WriteFile(outputPath, code, filePermission); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("autowire: generated %s\n", outputPath)
	return nil
}
