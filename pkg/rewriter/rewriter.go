package rewriter

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Config struct {
	PackagePath string
	TypeName    string
	OutputDir   string
	Recursive   bool // If true, recursively extract external type dependencies
}

type Rewriter struct {
	config         *Config
	pkg            *packages.Package
	fset           *token.FileSet
	neededDecls    map[string]*DeclInfo        // key: qualified type name (pkg.Type)
	neededImports  map[string]string           // key: package path, value: package name
	loadedPackages map[string]*packages.Package // cache of loaded packages
	externalTypes  map[string]bool              // types from external packages we need to extract
}

type DeclInfo struct {
	Name       string
	Decl       ast.Decl
	File       *ast.File
	Comment    *ast.CommentGroup
	PackagePath string // The package this declaration came from
}

func Rewrite(config *Config) error {
	r := &Rewriter{
		config:         config,
		fset:           token.NewFileSet(),
		neededDecls:    make(map[string]*DeclInfo),
		neededImports:  make(map[string]string),
		loadedPackages: make(map[string]*packages.Package),
		externalTypes:  make(map[string]bool),
	}

	// Load the package
	if err := r.loadPackage(); err != nil {
		return fmt.Errorf("failed to load package: %w", err)
	}

	// Find the target type and collect dependencies
	if err := r.collectDependencies(); err != nil {
		return fmt.Errorf("failed to collect dependencies: %w", err)
	}

	// Generate output files
	if err := r.generateOutput(); err != nil {
		return fmt.Errorf("failed to generate output: %w", err)
	}

	return nil
}

func (r *Rewriter) loadPackage() error {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Fset: r.fset,
	}

	pkgs, err := packages.Load(cfg, r.config.PackagePath)
	if err != nil {
		return err
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("package not found: %s", r.config.PackagePath)
	}

	r.pkg = pkgs[0]

	if len(r.pkg.Errors) > 0 {
		// Log errors but continue - the package might still be parseable
		for _, err := range r.pkg.Errors {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	return nil
}

func (r *Rewriter) collectDependencies() error {
	// Find the target type
	obj := r.pkg.Types.Scope().Lookup(r.config.TypeName)
	if obj == nil {
		return fmt.Errorf("type %s not found in package %s", r.config.TypeName, r.config.PackagePath)
	}

	// Find the declaration in the AST
	for _, file := range r.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == r.config.TypeName {
							// Found the target type, now collect it and its dependencies
							r.collectTypeDecl(typeSpec.Name.Name, genDecl, file)
							r.walkType(typeSpec.Type)

							// After collecting all types, scan for external imports used
							r.collectUsedImports()
							return nil
						}
					}
				}
			}
		}
	}

	return fmt.Errorf("type declaration not found in AST: %s", r.config.TypeName)
}

func (r *Rewriter) collectTypeDecl(name string, decl ast.Decl, file *ast.File) {
	if _, exists := r.neededDecls[name]; exists {
		return
	}

	genDecl := decl.(*ast.GenDecl)
	var comment *ast.CommentGroup
	if genDecl.Doc != nil {
		comment = genDecl.Doc
	}

	r.neededDecls[name] = &DeclInfo{
		Name:    name,
		Decl:    decl,
		File:    file,
		Comment: comment,
	}
}

func (r *Rewriter) walkType(expr ast.Expr) {
	switch t := expr.(type) {
	case *ast.Ident:
		// Check if this is a type from the same package
		if obj := r.pkg.Types.Scope().Lookup(t.Name); obj != nil {
			if _, ok := obj.Type().(*types.Named); ok {
				// Find and collect this type declaration
				r.findAndCollectType(t.Name)
			}
		}

	case *ast.StarExpr:
		r.walkType(t.X)

	case *ast.ArrayType:
		r.walkType(t.Elt)

	case *ast.MapType:
		r.walkType(t.Key)
		r.walkType(t.Value)

	case *ast.StructType:
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				r.walkType(field.Type)
			}
		}

	case *ast.SelectorExpr:
		// This is a type from another package (e.g., metav1.ObjectMeta)
		if ident, ok := t.X.(*ast.Ident); ok {
			// Record the import
			if obj := r.pkg.Types.Scope().Lookup(ident.Name); obj != nil {
				if pkgName, ok := obj.(*types.PkgName); ok {
					r.neededImports[pkgName.Imported().Path()] = pkgName.Name()
				}
			}
		}

	case *ast.InterfaceType:
		// Empty interface or interface type - generally safe

	case *ast.FuncType:
		// Function types - walk params and results
		if t.Params != nil {
			for _, field := range t.Params.List {
				r.walkType(field.Type)
			}
		}
		if t.Results != nil {
			for _, field := range t.Results.List {
				r.walkType(field.Type)
			}
		}

	case *ast.ChanType:
		r.walkType(t.Value)

	case *ast.Ellipsis:
		r.walkType(t.Elt)
	}
}

func (r *Rewriter) findAndCollectType(name string) {
	if _, exists := r.neededDecls[name]; exists {
		return // Already collected
	}

	// Search for the type declaration in all files
	for _, file := range r.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == name {
							r.collectTypeDecl(name, genDecl, file)
							// Recursively walk this type's dependencies
							r.walkType(typeSpec.Type)
							return
						}
					}
				}
			}
		}
	}
}

func (r *Rewriter) collectUsedImports() {
	// Build a map of package names to paths from all files
	pkgNameToPath := make(map[string]string)
	for _, file := range r.pkg.Syntax {
		for _, imp := range file.Imports {
			if imp.Path == nil {
				continue
			}
			// Remove quotes from import path
			path := strings.Trim(imp.Path.Value, `"`)

			var name string
			if imp.Name != nil {
				// Explicit import name
				name = imp.Name.Name
			} else {
				// Default package name (last component of path)
				name = filepath.Base(path)
			}
			pkgNameToPath[name] = path
		}
	}

	// Now walk through all collected declarations and find selector expressions
	for _, info := range r.neededDecls {
		ast.Inspect(info.Decl, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					// This might be a package reference (e.g., metav1.Time)
					if path, exists := pkgNameToPath[ident.Name]; exists {
						r.neededImports[path] = ident.Name
					}
				}
			}
			return true
		})
	}
}

func (r *Rewriter) generateOutput() error {
	// Create output directory structure
	pkgName := r.pkg.Name
	outputPath := filepath.Join(r.config.OutputDir, pkgName)
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return err
	}

	// Create a new file with the extracted types
	outputFile := filepath.Join(outputPath, "types.go")

	// Build the new AST file
	newFile := &ast.File{
		Name: ast.NewIdent(pkgName),
	}

	// Add package comment (don't set as Doc, we'll write it manually)
	packageComment := fmt.Sprintf("// Code generated by package-rewriter. DO NOT EDIT.\n// Source: %s\n", r.config.PackagePath)

	// Add imports
	if len(r.neededImports) > 0 {
		importDecl := &ast.GenDecl{
			Tok: token.IMPORT,
		}
		for path, name := range r.neededImports {
			importSpec := &ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, path),
				},
			}
			// Only add name if it's not the default package name
			if name != filepath.Base(path) && !strings.HasSuffix(path, "/"+name) {
				importSpec.Name = ast.NewIdent(name)
			}
			importDecl.Specs = append(importDecl.Specs, importSpec)
		}
		newFile.Decls = append(newFile.Decls, importDecl)
	}

	// Add type declarations
	for _, info := range r.neededDecls {
		newFile.Decls = append(newFile.Decls, info.Decl)
	}

	// Write the file
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write package comment first
	if _, err := f.WriteString(packageComment); err != nil {
		return err
	}

	if err := format.Node(f, r.fset, newFile); err != nil {
		return err
	}

	fmt.Printf("Generated: %s\n", outputFile)
	fmt.Printf("Extracted %d type(s)\n", len(r.neededDecls))
	fmt.Printf("Imports: %d\n", len(r.neededImports))

	return nil
}
