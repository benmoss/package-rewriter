module github.com/benmoss/package-rewriter/examples/argo_application

go 1.25.1

require (
	github.com/argoproj/argo-cd/v3 v3.2.0
	github.com/external-secrets/external-secrets/apis v0.0.0-20251118062813-5b49a903f879
)

require (
	github.com/argoproj/gitops-engine v0.7.1-0.20251006172252-b89b0871b414 // indirect
	k8s.io/apimachinery v0.34.1 // indirect
)

replace github.com/argoproj/gitops-engine => ./generated/github.com/argoproj/gitops-engine

replace github.com/argoproj/argo-cd/v3 => ./generated/github.com/argoproj/argo-cd/v3

replace github.com/external-secrets/external-secrets/apis => ./generated/github.com/external-secrets/external-secrets/apis

replace k8s.io/apimachinery => ./generated/k8s.io/apimachinery
