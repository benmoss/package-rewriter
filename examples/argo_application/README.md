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
- Add the `replace` directives to your go.mod

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
