package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/benmoss/package-rewriter/pkg/config"
	"github.com/benmoss/package-rewriter/pkg/rewriter"
)

func main() {
	var (
		configFile string
		pkgPath    string
		typeName   string
		outputDir  string
		verbosity  string
	)

	flag.StringVar(&configFile, "config", "", "Path to config file (YAML)")
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

	// Determine which mode to use: config file or CLI flags
	if configFile != "" {
		// Config file mode
		if err := runFromConfigFile(configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Legacy CLI mode
		if pkgPath == "" || typeName == "" {
			fmt.Fprintf(os.Stderr, "Usage:\n")
			fmt.Fprintf(os.Stderr, "  Config file mode: package-rewriter --config <config-file> [-v <level>]\n")
			fmt.Fprintf(os.Stderr, "  CLI mode:         package-rewriter --package <pkg> --type <type> [--output <dir>] [-v <level>]\n\n")
			flag.PrintDefaults()
			os.Exit(1)
		}

		cfg := &rewriter.Config{
			PackagePath: pkgPath,
			TypeName:    typeName,
			OutputDir:   outputDir,
		}

		if err := rewriter.RewriteRecursive(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully extracted %s from %s to %s\n", typeName, pkgPath, outputDir)
	}
}

func runFromConfigFile(configPath string) error {
	// Load config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Printf("Loaded config: %d package(s) to process\n", len(cfg.Packages))

	// Build list of all rewriter configs
	var rewriterConfigs []*rewriter.Config
	for _, pkgEntry := range cfg.Packages {
		for _, typeName := range pkgEntry.Types {
			rewriterConfigs = append(rewriterConfigs, &rewriter.Config{
				PackagePath: pkgEntry.Package,
				TypeName:    typeName,
				OutputDir:   cfg.Output,
			})
		}
	}

	fmt.Printf("Total types to extract: %d\n\n", len(rewriterConfigs))

	// Process all package/type pairs in a single batch
	if err := rewriter.RewriteRecursiveBatch(rewriterConfigs); err != nil {
		return fmt.Errorf("failed to process types: %w", err)
	}

	fmt.Printf("\n=== All packages processed successfully ===\n")
	fmt.Printf("Output directory: %s\n", cfg.Output)

	return nil
}
