# Package Rewriter

A Go CLI tool that extracts type definitions from Go packages by parsing the AST and pruning them to just the struct definitions and their dependencies. This is particularly useful when you need to use types from packages that have dependencies incompatible with your target platform (e.g., WASM).

## Problem

When building Go applications for specialized targets like `GOOS=wasip1 GOARCH=wasm`, you may encounter packages that have transitive dependencies which don't compile for those targets. For example, the ArgoCD API types package (`github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1`) has dependencies that fail to compile for WASM targets.

## Solution

This tool extracts just the type definitions you need (structs, type aliases, etc.) along with their dependencies, without pulling in the problematic runtime code or incompatible dependencies. It:

1. Parses the Go AST of the target package
2. Finds the requested type and recursively identifies all dependent types
3. Filters out non-type declarations (functions, vars, consts)
4. Generates new Go files with only the extracted declarations
5. Includes only the necessary imports

## Installation

```bash
go install github.com/benmoss/package-rewriter@latest
```

Or build from source:

```bash
git clone https://github.com/benmoss/package-rewriter.git
cd package-rewriter
go build
```

## Usage

```bash
package-rewriter --package <package-path> --type <type-name> [--output <output-dir>]
```

### Options

- `--package`: Package path to extract from (required)
- `--type`: Type name to extract (required)
- `--output`: Output directory for generated code (default: `./generated`)
- `-v`: Log level: `debug`, `info`, `warn`, `error` (default: `info`)

### Example

Extract the `Application` type from ArgoCD:

```bash
package-rewriter \
  --package github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1 \
  --type Application \
  --output ./generated
```

This will:
- **Recursively** extract the `Application` type and all its dependencies across multiple packages
- Extract 69 types from the v1alpha1 package
- Extract external types from k8s.io/apimachinery and gitops-engine packages
- Generate code to separate directories for each package

Output:
```
Processing: github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1.Application
Processing: github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1.ApplicationSpec
...
Processing: k8s.io/apimachinery/pkg/runtime.RawExtension
Processing: github.com/argoproj/gitops-engine/pkg/health.HealthStatusCode
...

Generating output for 5 packages...
Generated: generated/github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1/types.go (69 types)
Generated: generated/github.com/argoproj/gitops-engine/pkg/health/types.go (1 types)
Generated: generated/k8s.io/apimachinery/pkg/runtime/types.go (2 types)
Generated: generated/k8s.io/apimachinery/pkg/runtime/schema/types.go (2 types)
Generated: generated/k8s.io/apimachinery/pkg/util/intstr/types.go (2 types)

Successfully extracted Application from github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1 to ./generated
```

## How It Works

The tool performs **fully recursive type extraction** across package boundaries:

1. **Load Target Package**: Uses `golang.org/x/tools/go/packages` to load the package with full type information
2. **Find Target Type**: Locates the requested type declaration in the AST
3. **Walk Type Dependencies**: Analyzes the type structure to find dependencies:
   - Struct fields and their types
   - Embedded types
   - Type aliases
   - External package references (e.g., `metav1.Time`, `health.HealthStatus`)
4. **Queue External Types**: When external types are found, they're added to the extraction queue
5. **Recursively Process**: For each queued type:
   - Load the external package
   - Extract the type definition
   - Walk its dependencies
   - Queue any new external types found
6. **Continue Until Complete**: Repeat until all types are extracted or only stdlib types remain
7. **Generate Output**: Create separate type files for each package with proper imports
8. **Update go.mod**: Automatically write `replace` directives to your go.mod file

## Using the Generated Code

After extraction, **the tool automatically updates your `go.mod`** with the necessary `replace` directives:

```
======================================================================
Add these replace directives to your go.mod:
======================================================================

replace github.com/argoproj/argo-cd/v3 => ./generated/github.com/argoproj/argo-cd/v3
replace github.com/argoproj/gitops-engine => ./generated/github.com/argoproj/gitops-engine
replace k8s.io/apimachinery => ./generated/k8s.io/apimachinery

======================================================================

Updated go.mod with 3 replace directive(s)
```

The tool will:
- Find your `go.mod` file (in the current directory or parent directories)
- Remove any existing replace directives for the generated modules
- Add new replace directives pointing to the generated code
- Save the updated `go.mod`

Then you can use the types normally in your code:

```go
import v1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

app := &v1alpha1.Application{
    Spec: v1alpha1.ApplicationSpec{
        Project: "my-project",
        Destination: v1alpha1.ApplicationDestination{
            Server:    "https://kubernetes.default.svc",
            Namespace: "default",
        },
    },
}
```

Go will automatically use your generated lightweight versions instead of the full packages!

## Limitations

- Only extracts type definitions (structs, type aliases, interfaces)
- Does not extract functions, methods, or constants
- Extracted types from external packages may still have their own incompatible dependencies
- Method sets on types are not preserved

## Use Cases

1. **WASM Compatibility**: Extract types from packages with WASM-incompatible dependencies
2. **Dependency Reduction**: Create lightweight versions of heavy packages when you only need the types
3. **API Client Generation**: Extract just the API types without pulling in server-side dependencies
4. **Cross-Compilation**: Work around platform-specific dependencies in type-only scenarios

## Future Enhancements

- [ ] Support for extracting multiple types in one run
- [ ] Option to include constants and enums
- [x] Automatic `go.mod` replace directive generation
- [ ] Support for extracting entire package hierarchies
- [ ] Dead code elimination for unused fields in complex dependency graphs

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

MIT License - see LICENSE file for details
