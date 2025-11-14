package rewriter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestCollectSourceImports(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected map[string]string
	}{
		{
			name: "simple alias import",
			source: `package test
import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)`,
			expected: map[string]string{
				"k8s.io/apimachinery/pkg/apis/meta/v1": "metav1",
			},
		},
		{
			name: "synccommon alias import",
			source: `package test
import (
	synccommon "github.com/argoproj/gitops-engine/pkg/sync/common"
)`,
			expected: map[string]string{
				"github.com/argoproj/gitops-engine/pkg/sync/common": "synccommon",
			},
		},
		{
			name: "mangled name should be skipped",
			source: `package test
import (
	github_com_argoproj_gitops_engine_pkg_sync_common "github.com/argoproj/gitops-engine/pkg/sync/common"
)`,
			expected: map[string]string{
				// Should not include the mangled name
			},
		},
		{
			name: "both mangled and real alias - prefer real",
			source: `package test
import (
	github_com_argoproj_gitops_engine_pkg_sync_common "github.com/argoproj/gitops-engine/pkg/sync/common"
	synccommon "github.com/argoproj/gitops-engine/pkg/sync/common"
)`,
			expected: map[string]string{
				"github.com/argoproj/gitops-engine/pkg/sync/common": "synccommon",
			},
		},
		{
			name: "no alias - use base name",
			source: `package test
import (
	"github.com/example/pkg/common"
)`,
			expected: map[string]string{
				"github.com/example/pkg/common": "common",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.source, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("Failed to parse source: %v", err)
			}

			r := &RecursiveRewriter{
				fset: fset,
			}

			pkgInfo := &PackageInfo{
				SourceImports: make(map[string]string),
			}

			r.collectSourceImports(pkgInfo, file)

			// Check results
			if len(pkgInfo.SourceImports) != len(tt.expected) {
				t.Errorf("Expected %d imports, got %d: %v", len(tt.expected), len(pkgInfo.SourceImports), pkgInfo.SourceImports)
			}

			for path, expectedName := range tt.expected {
				if gotName, ok := pkgInfo.SourceImports[path]; !ok {
					t.Errorf("Expected import %s not found", path)
				} else if gotName != expectedName {
					t.Errorf("For path %s: expected name %s, got %s", path, expectedName, gotName)
				}
			}

			// Check no unexpected imports
			for path, name := range pkgInfo.SourceImports {
				if expectedName, ok := tt.expected[path]; !ok {
					t.Errorf("Unexpected import: %s -> %s", path, name)
				} else if name != expectedName {
					t.Errorf("For path %s: expected name %s, got %s", path, expectedName, name)
				}
			}
		})
	}
}

func TestWalkTypeForDeps_SelectorExpr(t *testing.T) {
	tests := []struct {
		name            string
		typeSource      string
		sourceImports   map[string]string
		expectedQueue   []TypeRef
		expectedImports map[string]string
	}{
		{
			name: "metav1.Time should queue Time type",
			typeSource: `package test
type Foo struct {
	Time metav1.Time
}`,
			sourceImports: map[string]string{
				"k8s.io/apimachinery/pkg/apis/meta/v1": "metav1",
			},
			expectedQueue: []TypeRef{
				{PackagePath: "k8s.io/apimachinery/pkg/apis/meta/v1", TypeName: "Time"},
			},
			expectedImports: map[string]string{
				"k8s.io/apimachinery/pkg/apis/meta/v1": "metav1",
			},
		},
		{
			name: "synccommon.OperationPhase should queue OperationPhase",
			typeSource: `package test
type Foo struct {
	Phase synccommon.OperationPhase
}`,
			sourceImports: map[string]string{
				"github.com/argoproj/gitops-engine/pkg/sync/common": "synccommon",
			},
			expectedQueue: []TypeRef{
				{PackagePath: "github.com/argoproj/gitops-engine/pkg/sync/common", TypeName: "OperationPhase"},
			},
			expectedImports: map[string]string{
				"github.com/argoproj/gitops-engine/pkg/sync/common": "synccommon",
			},
		},
		{
			name: "embedded metav1.TypeMeta should queue TypeMeta",
			typeSource: `package test
type Foo struct {
	metav1.TypeMeta
}`,
			sourceImports: map[string]string{
				"k8s.io/apimachinery/pkg/apis/meta/v1": "metav1",
			},
			expectedQueue: []TypeRef{
				{PackagePath: "k8s.io/apimachinery/pkg/apis/meta/v1", TypeName: "TypeMeta"},
			},
			expectedImports: map[string]string{
				"k8s.io/apimachinery/pkg/apis/meta/v1": "metav1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.typeSource, 0)
			if err != nil {
				t.Fatalf("Failed to parse source: %v", err)
			}

			r := &RecursiveRewriter{
				fset:           fset,
				pendingTypes:   []TypeRef{},
				processedTypes: make(map[string]bool),
			}

			// Create a mock package
			pkgInfo := &PackageInfo{
				Pkg: &packages.Package{
					PkgPath: "test",
					Imports: make(map[string]*packages.Package),
					Types:   nil, // We won't check same-package types in this test
				},
				Imports:       make(map[string]string),
				SourceImports: tt.sourceImports,
			}

			// Find the struct type and walk it
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							r.walkTypeForDeps(pkgInfo, typeSpec.Type)
						}
					}
				}
			}

			// Check queued types
			if len(r.pendingTypes) != len(tt.expectedQueue) {
				t.Errorf("Expected %d queued types, got %d: %v", len(tt.expectedQueue), len(r.pendingTypes), r.pendingTypes)
			}

			for i, expected := range tt.expectedQueue {
				if i >= len(r.pendingTypes) {
					break
				}
				got := r.pendingTypes[i]
				if got.PackagePath != expected.PackagePath || got.TypeName != expected.TypeName {
					t.Errorf("Queue[%d]: expected %v, got %v", i, expected, got)
				}
			}

			// Check imports were recorded
			for path, expectedName := range tt.expectedImports {
				if gotName, ok := pkgInfo.Imports[path]; !ok {
					t.Errorf("Expected import %s not recorded in Imports", path)
				} else if gotName != expectedName {
					t.Errorf("For import %s: expected name %s, got %s", path, expectedName, gotName)
				}
			}
		})
	}
}
