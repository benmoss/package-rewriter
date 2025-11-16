package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/benmoss/package-rewriter/pkg/rewriter"
)

func main() {
	var (
		pkgPath   string
		typeName  string
		outputDir string
		verbosity string
	)

	flag.StringVar(&pkgPath, "package", "", "Package path to extract from (e.g., github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1)")
	flag.StringVar(&typeName, "type", "", "Type name to extract (e.g., Application)")
	flag.StringVar(&outputDir, "output", "./generated", "Output directory for generated code")
	flag.StringVar(&verbosity, "v", "info", "Log level: debug, info, warn, error")

	flag.Parse()

	// Configure slog based on verbosity flag
	var level slog.Level
	switch verbosity {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "Invalid verbosity level: %s (use: debug, info, warn, error)\n", verbosity)
		os.Exit(1)
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))

	if pkgPath == "" || typeName == "" {
		fmt.Fprintf(os.Stderr, "Usage: package-rewriter --package <pkg> --type <type> [--output <dir>] [-v <level>]\n")
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
