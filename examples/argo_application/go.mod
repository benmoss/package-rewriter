module github.com/benmoss/package-rewriter/examples/argo_application

go 1.25.0

require github.com/argoproj/argo-cd/v3 v3.2.0

require (
	github.com/argoproj/gitops-engine v0.7.1-0.20251006172252-b89b0871b414 // indirect
	k8s.io/apimachinery v0.34.0 // indirect
)

replace github.com/argoproj/argo-cd/v3 => ./generated/github.com/argoproj/argo-cd/v3

replace k8s.io/apimachinery => ./generated/k8s.io/apimachinery

replace github.com/argoproj/gitops-engine => ./generated/github.com/argoproj/gitops-engine
