package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/benmoss/package-rewriter/pkg/rewriter"
)

func main() {
	var (
		pkgPath   string
		typeName  string
		outputDir string
	)

	flag.StringVar(&pkgPath, "package", "", "Package path to extract from (e.g., github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1)")
	flag.StringVar(&typeName, "type", "", "Type name to extract (e.g., Application)")
	flag.StringVar(&outputDir, "output", "./generated", "Output directory for generated code")

	flag.Parse()

	if pkgPath == "" || typeName == "" {
		fmt.Fprintf(os.Stderr, "Usage: package-rewriter --package <pkg> --type <type> [--output <dir>]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	config := &rewriter.Config{
		PackagePath: pkgPath,
		TypeName:    typeName,
		OutputDir:   outputDir,
	}

	// Always use recursive extraction
	if err := rewriter.RewriteRecursive(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully extracted %s from %s to %s\n", typeName, pkgPath, outputDir)
}
