# ArgoCD Application Type Example

This example demonstrates using `package-rewriter` to extract the ArgoCD `Application` type and use it with a `replace` directive.

## What This Example Shows

1. **Extracting types**: Using `package-rewriter` to extract just the type definitions from ArgoCD's v1alpha1 package
2. **Using replace directives**: Pointing the import to our generated lightweight version instead of the full package
3. **Reduced dependencies**: The generated package has only 5 imports vs the full ArgoCD package's 148 indirect dependencies

## Running the Example

### Generate the Types

From this directory, run:

```bash
../../package-rewriter \
  --package github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1 \
  --type Application \
  --output ./generated
```

This will:
- **Recursively** extract the `Application` type and all dependencies
- Extract 69 types from the v1alpha1 package
- Extract types from 4 external packages (gitops-engine, k8s.io/apimachinery)
- Generate code to separate directories for each package
- Print `replace` directives for your go.mod

### Build and Run

```bash
go build -o example
./example
```

Output:
```json
Application Spec:
{
  "destination": {
    "server": "https://kubernetes.default.svc",
    "namespace": "default"
  },
  "project": "my-project"
}
```

## How the Replace Directives Work

After running the tool, copy the printed `replace` directives into your [go.mod](go.mod):

```go
// From tool output
replace github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1 => ./generated/github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1
replace github.com/argoproj/gitops-engine/pkg/health => ./generated/github.com/argoproj/gitops-engine/pkg/health
replace k8s.io/apimachinery/pkg/runtime => ./generated/k8s.io/apimachinery/pkg/runtime
replace k8s.io/apimachinery/pkg/runtime/schema => ./generated/k8s.io/apimachinery/pkg/runtime/schema
replace k8s.io/apimachinery/pkg/util/intstr => ./generated/k8s.io/apimachinery/pkg/util/intstr
```

These tell Go to use our generated lightweight versions instead of the full packages. Your code imports packages normally:

```go
import v1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
```

But Go automatically redirects to the generated versions!

## Generated Package Structure

```
generated/
└── v1alpha1/
    ├── go.mod       # Declares only the dependencies actually needed by the types
    └── types.go     # 69 extracted type definitions with 5 imports
```

The generated `go.mod` has the correct module path to match what's being replaced:

```go
module github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1
```

## Dependency Comparison

### Original ArgoCD Package
- **Direct dependencies**: Full ArgoCD v3.2.0
- **Indirect dependencies**: 148 packages
- **Includes**: Server code, git clients, authentication, etc.

### Generated Package
- **Direct dependencies**:
  - `github.com/argoproj/gitops-engine` (for health types)
  - `k8s.io/apimachinery` (for metav1.Time, intstr, runtime)
- **Indirect dependencies**: ~90 packages (from k8s dependencies)
- **Includes**: Only type definitions

## WASM Compatibility Note

While this approach significantly reduces dependencies, some of the remaining dependencies (particularly from k8s.io and gitops-engine) still have transitive dependencies that are not WASM-compatible (like `github.com/moby/term` and `k8s.io/kubectl`).

For full WASM compatibility, you would need to:
1. Further extract just the types you need (e.g., only `ApplicationSpec` without `Application`)
2. Create stub types for external dependencies like `metav1.Time`
3. Use build tags to exclude problematic dependencies

This tool provides the foundation for that work by making it easy to extract and iterate on the minimal set of types needed.
