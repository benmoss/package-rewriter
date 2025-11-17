package rewriter

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Config holds the configuration for the package rewriter
type Config struct {
	PackagePath string
	TypeName    string
	OutputDir   string
}

// DeclInfo holds information about a type declaration
type DeclInfo struct {
	Name        string
	Decl        ast.Decl
	File        *ast.File
	Comment     *ast.CommentGroup
	PackagePath string // The package this declaration came from
}

// RecursiveRewriter handles recursive extraction of types across packages
type RecursiveRewriter struct {
	config         *Config
	fset           *token.FileSet
	packages       map[string]*PackageInfo // key: package path
	pendingTypes   []TypeRef               // types we need to extract
	processedTypes map[string]bool         // types we've already extracted
	stdlib         map[string]bool         // stdlib packages to skip
	modules        map[string]*ModuleInfo  // key: module path
}

// ModuleInfo holds information about a Go module
type ModuleInfo struct {
	Path     string   // module path (e.g., "github.com/argoproj/argo-cd/v3")
	Packages []string // package paths in this module
}

// PackageInfo holds information about a package being processed
type PackageInfo struct {
	Pkg           *packages.Package
	Decls         map[string]*DeclInfo // key: type name
	Imports       map[string]string    // key: package path, value: package name (imports actually used in generated code)
	SourceImports map[string][]string  // key: package path, value: all package names/aliases used across source files
	NameToPath    map[string]string    // key: package name/alias, value: package path (reverse lookup)
	OutputSubdir  string               // subdirectory in output (e.g., "k8s.io/apimachinery/pkg/apis/meta/v1")
	ModulePath    string               // module this package belongs to
}

// TypeRef represents a reference to a type we need to extract
type TypeRef struct {
	PackagePath string
	TypeName    string
}

func (tr TypeRef) String() string {
	return fmt.Sprintf("%s.%s", tr.PackagePath, tr.TypeName)
}

func RewriteRecursive(config *Config) error {
	r := &RecursiveRewriter{
		config:         config,
		fset:           token.NewFileSet(),
		packages:       make(map[string]*PackageInfo),
		processedTypes: make(map[string]bool),
		stdlib:         makeStdlibMap(),
		modules:        make(map[string]*ModuleInfo),
	}

	// Start with the target type
	r.pendingTypes = append(r.pendingTypes, TypeRef{
		PackagePath: config.PackagePath,
		TypeName:    config.TypeName,
	})

	// Find and load go.mod
	goModPath, err := FindGoMod()
	var goMod *GoModManager
	if err != nil {
		slog.Warn("go.mod not found, replace directives will not be managed automatically", "error", err)
	} else {
		goMod, err = NewGoModManager(goModPath)
		if err != nil {
			slog.Warn("Failed to parse go.mod, replace directives will not be managed automatically", "error", err)
			goMod = nil
		} else {
			// Remove existing replace directives for all modules (we'll add back only what we generate)
			replaces := goMod.GetReplaces()
			if len(replaces) > 0 {
				slog.Info("Removing existing replace directives from go.mod", "count", len(replaces))
				for modulePath := range replaces {
					if err := goMod.RemoveReplace(modulePath); err != nil {
						slog.Warn("Failed to remove replace directive", "module", modulePath, "error", err)
					}
				}
				if err := goMod.Save(); err != nil {
					slog.Warn("Failed to save go.mod after removing replace directives", "error", err)
				}
			}
		}
	}

	// Process types recursively
	for len(r.pendingTypes) > 0 {
		// Pop next type to process
		typeRef := r.pendingTypes[0]
		r.pendingTypes = r.pendingTypes[1:]

		// Skip if already processed
		if r.processedTypes[typeRef.String()] {
			continue
		}

		// Skip stdlib types
		if r.isStdlib(typeRef.PackagePath) {
			r.processedTypes[typeRef.String()] = true
			continue
		}

		fmt.Printf("Processing: %s\n", typeRef.String())

		// Extract this type and queue its dependencies
		if err := r.extractType(typeRef); err != nil {
			return fmt.Errorf("failed to extract %s: %w", typeRef.String(), err)
		}

		r.processedTypes[typeRef.String()] = true
	}

	// Generate output for all packages
	if err := r.generateOutput(); err != nil {
		return err
	}

	// Add replace directives for generated modules
	if goMod != nil {
		return r.updateGoModReplaces(goMod)
	}

	return nil
}

func (r *RecursiveRewriter) extractType(typeRef TypeRef) error {
	// Load package if not already loaded
	pkgInfo, err := r.loadPackageInfo(typeRef.PackagePath)
	if err != nil {
		return err
	}

	// Find the type declaration in the package
	found := false
	var typeSpec *ast.TypeSpec
	var genDecl *ast.GenDecl
	var file *ast.File

	for _, f := range pkgInfo.Pkg.Syntax {
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if ts.Name.Name == typeRef.TypeName {
							// Found it!
							typeSpec = ts
							genDecl = gd
							file = f
							found = true
							break
						}
					}
				}
				if found {
					break
				}
			}
		}
		if found {
			break
		}
	}

	if found {
		// Store the declaration
		r.collectTypeDecl(pkgInfo, typeSpec.Name.Name, genDecl, file)

		// Walk the type to find dependencies
		r.walkTypeForDeps(pkgInfo, typeSpec.Type)
	}

	if !found {
		return fmt.Errorf("type %s not found in package %s", typeRef.TypeName, typeRef.PackagePath)
	}

	return nil
}

func (r *RecursiveRewriter) loadPackageInfo(pkgPath string) (*PackageInfo, error) {
	if pkgInfo, exists := r.packages[pkgPath]; exists {
		return pkgInfo, nil
	}

	// Load the package
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedModule,
		Fset: r.fset,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package not found: %s", pkgPath)
	}

	pkg := pkgs[0]

	if len(pkg.Errors) > 0 {
		for _, err := range pkg.Errors {
			slog.Warn("Error loading package", "path", pkgPath, "error", err)
		}
	}

	// Get the module path for this package
	modulePath := getModulePath(pkg)

	// Track the module
	if _, exists := r.modules[modulePath]; !exists {
		r.modules[modulePath] = &ModuleInfo{
			Path:     modulePath,
			Packages: []string{},
		}
	}
	r.modules[modulePath].Packages = append(r.modules[modulePath].Packages, pkgPath)

	// Create package info
	pkgInfo := &PackageInfo{
		Pkg:           pkg,
		Decls:         make(map[string]*DeclInfo),
		Imports:       make(map[string]string),
		SourceImports: make(map[string][]string),
		NameToPath:    make(map[string]string),
		OutputSubdir:  pkgPath,
		ModulePath:    modulePath,
	}

	// Collect all imports from source files for name resolution
	slog.Debug("Loading package",
		"path", pkgPath,
		"goFiles", len(pkg.GoFiles),
		"compiledGoFiles", len(pkg.CompiledGoFiles),
		"syntaxFiles", len(pkg.Syntax))

	for _, file := range pkg.Syntax {
		r.collectSourceImports(pkgInfo, file)
	}

	slog.Debug("Collected source imports",
		"path", pkgPath,
		"importCount", len(pkgInfo.SourceImports))

	r.packages[pkgPath] = pkgInfo
	return pkgInfo, nil
}

func (r *RecursiveRewriter) collectSourceImports(pkgInfo *PackageInfo, file *ast.File) {
	// Scan the file's imports and add them to SourceImports for lookup
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		// Remove quotes from path
		path := imp.Path.Value[1 : len(imp.Path.Value)-1]

		// Determine the package name (either from alias or last component)
		var pkgName string
		hasExplicitAlias := false
		isMangled := false
		if imp.Name != nil {
			pkgName = imp.Name.Name
			hasExplicitAlias = true

			// Detect auto-generated mangled names by checking if the alias contains
			// multiple consecutive package path components separated by underscores.
			// For example: "github_com_argoproj_gitops_engine_pkg_sync_common"
			// Real user aliases like "synccommon", "metav1", "v1alpha1" don't match this pattern.
			pathParts := strings.Split(strings.Trim(path, "/"), "/")
			if len(pathParts) >= 3 {
				// Check if the alias contains at least 3 path components joined by underscores
				mangledPattern := strings.Join(pathParts, "_")
				mangledPattern = strings.ReplaceAll(mangledPattern, ".", "_")
				mangledPattern = strings.ReplaceAll(mangledPattern, "-", "_")
				if strings.Contains(pkgName, mangledPattern) ||
					(len(pathParts) >= 3 && strings.Count(pkgName, "_") >= 2) {
					isMangled = true
				}
			}
		} else {
			pkgName = filepath.Base(path)
		}

		// Skip mangled import names
		if isMangled {
			slog.Debug("Skipping mangled import name",
				"path", path,
				"mangledName", pkgName)
			continue
		}

		// Add to SourceImports (all aliases) and NameToPath (reverse lookup)
		// Check if this name/alias already exists for this path
		alreadyExists := false
		for _, existingName := range pkgInfo.SourceImports[path] {
			if existingName == pkgName {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists {
			pkgInfo.SourceImports[path] = append(pkgInfo.SourceImports[path], pkgName)

			// Build reverse map: name -> path
			// If the same name maps to different paths, prefer explicit aliases
			if existingPath, exists := pkgInfo.NameToPath[pkgName]; exists {
				// Name conflict - prefer explicit alias over inferred
				if hasExplicitAlias {
					pkgInfo.NameToPath[pkgName] = path
					slog.Debug("Name conflict - preferring explicit alias",
						"name", pkgName,
						"oldPath", existingPath,
						"newPath", path)
				}
			} else {
				pkgInfo.NameToPath[pkgName] = path
			}
		}
	}
}

func (r *RecursiveRewriter) collectTypeDecl(pkgInfo *PackageInfo, name string, decl *ast.GenDecl, file *ast.File) {
	if _, exists := pkgInfo.Decls[name]; exists {
		return
	}

	var comment *ast.CommentGroup
	if decl.Doc != nil {
		comment = decl.Doc
	}

	pkgInfo.Decls[name] = &DeclInfo{
		Name:        name,
		Decl:        decl,
		File:        file,
		Comment:     comment,
		PackagePath: pkgInfo.Pkg.PkgPath,
	}
}

func (r *RecursiveRewriter) walkTypeForDeps(pkgInfo *PackageInfo, expr ast.Expr) {
	if expr == nil {
		return
	}

	switch t := expr.(type) {
	case *ast.Ident:
		// Check if this is a type from the same package
		if obj := pkgInfo.Pkg.Types.Scope().Lookup(t.Name); obj != nil {
			// Check if this is a type name (includes both named types and type aliases)
			if _, ok := obj.(*types.TypeName); ok {
				// Need to extract this type from the same package
				r.queueType(pkgInfo.Pkg.PkgPath, t.Name)
			}
		}

	case *ast.StarExpr:
		r.walkTypeForDeps(pkgInfo, t.X)

	case *ast.ArrayType:
		r.walkTypeForDeps(pkgInfo, t.Elt)

	case *ast.MapType:
		r.walkTypeForDeps(pkgInfo, t.Key)
		r.walkTypeForDeps(pkgInfo, t.Value)

	case *ast.StructType:
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				r.walkTypeForDeps(pkgInfo, field.Type)
			}
		}

	case *ast.SelectorExpr:
		// This is a type from another package (e.g., metav1.Time, synccommon.OperationPhase)
		if ident, ok := t.X.(*ast.Ident); ok {
			// Look up the package using the name (reverse lookup)
			pkgName := ident.Name
			var externalPkgPath string

			// Use NameToPath for direct reverse lookup
			if path, exists := pkgInfo.NameToPath[pkgName]; exists {
				externalPkgPath = path
			}

			// If not found, check all imported packages from the loader
			if externalPkgPath == "" {
				for path, imp := range pkgInfo.Pkg.Imports {
					if imp.Name == pkgName {
						externalPkgPath = path
						break
					}
				}
			}

			// If still not found, check our Imports map (already used imports)
			if externalPkgPath == "" {
				for path, name := range pkgInfo.Imports {
					if name == pkgName {
						externalPkgPath = path
						break
					}
				}
			}

			if externalPkgPath != "" {
				typeName := t.Sel.Name
				// Queue this external type for extraction
				r.queueType(externalPkgPath, typeName)

				// Record the import for this package with the correct alias
				pkgInfo.Imports[externalPkgPath] = pkgName
			}
		}

	case *ast.InterfaceType:
		// Interface - might have embedded interfaces
		if t.Methods != nil {
			for _, field := range t.Methods.List {
				r.walkTypeForDeps(pkgInfo, field.Type)
			}
		}

	case *ast.FuncType:
		if t.Params != nil {
			for _, field := range t.Params.List {
				r.walkTypeForDeps(pkgInfo, field.Type)
			}
		}
		if t.Results != nil {
			for _, field := range t.Results.List {
				r.walkTypeForDeps(pkgInfo, field.Type)
			}
		}

	case *ast.ChanType:
		r.walkTypeForDeps(pkgInfo, t.Value)

	case *ast.Ellipsis:
		r.walkTypeForDeps(pkgInfo, t.Elt)

	}
}

func (r *RecursiveRewriter) queueType(pkgPath, typeName string) {
	typeRef := TypeRef{
		PackagePath: pkgPath,
		TypeName:    typeName,
	}

	// Skip if already processed or queued
	if r.processedTypes[typeRef.String()] {
		return
	}

	// Check if already in queue
	for _, pending := range r.pendingTypes {
		if pending.String() == typeRef.String() {
			return
		}
	}

	r.pendingTypes = append(r.pendingTypes, typeRef)
}

func (r *RecursiveRewriter) generateOutput() error {
	fmt.Printf("\nGenerating output for %d packages...\n", len(r.packages))

	// First, create go.mod files for each module
	if err := r.generateModuleFiles(); err != nil {
		return err
	}

	for pkgPath, pkgInfo := range r.packages {
		if len(pkgInfo.Decls) == 0 {
			continue
		}

		// Create output directory
		outputPath := filepath.Join(r.config.OutputDir, pkgInfo.OutputSubdir)
		if err := os.MkdirAll(outputPath, 0o755); err != nil {
			return err
		}

		// Generate the types file
		outputFile := filepath.Join(outputPath, "types.go")

		// Build AST file
		newFile := &ast.File{
			Name: ast.NewIdent(pkgInfo.Pkg.Name),
		}

		// Add package comment
		packageComment := fmt.Sprintf("// Code generated by package-rewriter. DO NOT EDIT.\n// Source: %s\n", pkgPath)

		// Add imports (only used imports from this package's perspective)
		if len(pkgInfo.Imports) > 0 {
			importDecl := &ast.GenDecl{
				Tok: token.IMPORT,
			}
			for path, name := range pkgInfo.Imports {
				// Only add import if we actually generated that package
				if _, exists := r.packages[path]; !exists && !r.isStdlib(path) {
					continue // Skip imports to packages we didn't extract
				}

				importSpec := &ast.ImportSpec{
					Path: &ast.BasicLit{
						Kind:  token.STRING,
						Value: fmt.Sprintf(`"%s"`, path),
					},
				}
				if name != filepath.Base(path) && !strings.HasSuffix(path, "/"+name) {
					importSpec.Name = ast.NewIdent(name)
				}
				importDecl.Specs = append(importDecl.Specs, importSpec)
			}
			if len(importDecl.Specs) > 0 {
				newFile.Decls = append(newFile.Decls, importDecl)
			}
		}

		// Add type declarations
		for _, info := range pkgInfo.Decls {
			newFile.Decls = append(newFile.Decls, info.Decl)
		}

		// Write the file
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.WriteString(packageComment); err != nil {
			return err
		}

		if err := format.Node(f, r.fset, newFile); err != nil {
			return err
		}

		fmt.Printf("Generated: %s (%d types)\n", outputFile, len(pkgInfo.Decls))
	}

	return nil
}

func (r *RecursiveRewriter) generateModuleFiles() error {
	for modulePath, moduleInfo := range r.modules {
		// Skip stdlib modules
		if r.isStdlib(modulePath) {
			continue
		}

		// Check if any packages in this module have declarations
		hasDecls := false
		for _, pkgPath := range moduleInfo.Packages {
			if pkgInfo, exists := r.packages[pkgPath]; exists && len(pkgInfo.Decls) > 0 {
				hasDecls = true
				break
			}
		}
		if !hasDecls {
			continue
		}

		// Create module directory
		moduleDir := filepath.Join(r.config.OutputDir, modulePath)
		if err := os.MkdirAll(moduleDir, 0o755); err != nil {
			return err
		}

		// Generate go.mod file
		goModPath := filepath.Join(moduleDir, "go.mod")
		goModContent := fmt.Sprintf("module %s\n\ngo 1.21\n", modulePath)

		if err := os.WriteFile(goModPath, []byte(goModContent), 0o644); err != nil {
			return err
		}

		fmt.Printf("Generated: %s\n", goModPath)
	}
	return nil
}

func (r *RecursiveRewriter) updateGoModReplaces(goMod *GoModManager) error {
	// Get list of modules with generated code
	var modulePaths []string
	for modulePath := range r.modules {
		if r.isStdlib(modulePath) {
			continue
		}
		// Check if module has any declarations
		hasDecls := false
		for _, pkgPath := range r.modules[modulePath].Packages {
			if pkgInfo, exists := r.packages[pkgPath]; exists && len(pkgInfo.Decls) > 0 {
				hasDecls = true
				break
			}
		}
		if hasDecls {
			modulePaths = append(modulePaths, modulePath)
		}
	}

	// Add replace directives
	for _, modulePath := range modulePaths {
		relPath := filepath.Join(r.config.OutputDir, modulePath)
		// Ensure path starts with ./ for go.mod replace directive
		if !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, ".") {
			relPath = "./" + relPath
		}
		if err := goMod.AddReplace(modulePath, relPath); err != nil {
			return fmt.Errorf("failed to add replace directive for %s: %w", modulePath, err)
		}
		slog.Info("Added replace directive", "module", modulePath, "path", relPath)
	}

	// Save go.mod
	if err := goMod.Save(); err != nil {
		return fmt.Errorf("failed to save go.mod: %w", err)
	}

	fmt.Printf("\nUpdated go.mod with %d replace directive(s)\n", len(modulePaths))
	return nil
}

func (r *RecursiveRewriter) isStdlib(pkgPath string) bool {
	// Simple heuristic: stdlib packages don't have a domain in the path
	return !strings.Contains(pkgPath, ".")
}

// getModulePath extracts the module path from a package path
// For example: "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1" -> "github.com/argoproj/argo-cd/v3"
func getModulePath(pkg *packages.Package) string {
	if pkg.Module != nil {
		return pkg.Module.Path
	}
	// Fallback: try to infer from package path
	// This is a heuristic and may not work for all cases
	return pkg.PkgPath
}

func makeStdlibMap() map[string]bool {
	// Common stdlib packages
	return map[string]bool{
		"fmt":     true,
		"strings": true,
		"time":    true,
		"errors":  true,
		"io":      true,
		"os":      true,
		"path":    true,
		"sort":    true,
		"sync":    true,
		// Add more as needed
	}
}
