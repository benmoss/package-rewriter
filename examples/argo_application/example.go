package main

import (
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
)

func main() {
	// Create an Application spec
	app := v1alpha1.ApplicationSpec{
		Project: "my-project",
		Destination: v1alpha1.ApplicationDestination{
			Server:    "https://kubernetes.default.svc",
			Namespace: "default",
		},
	}

	authConfig := v1alpha1.AWSAuthConfig{
		RoleARN:     "arn:aws:iam::123456789012:role/MyRole",
		ClusterName: "my-eks-cluster",
	}

	githubProvider := v1beta1.GithubProvider{
		AppID:        123456,
		Organization: "benmoss",
		Repository:   "package-rewriter",
		Environment:  "abc",
	}
	secretStoreSpec := v1beta1.SecretStoreSpec{
		Provider: &v1beta1.SecretStoreProvider{
			Github: &githubProvider,
		},
	}

	for _, obj := range []struct {
		name string
		obj  interface{}
	}{{"Application Spec", app}, {"AWSAuthConfig", authConfig}, {"GithubProvider", githubProvider}, {"SecretStoreSpec", secretStoreSpec}} {
		// Demonstrate JSON marshaling (common use case)
		data, err := json.MarshalIndent(obj.obj, "", "  ")
		if err != nil {
			panic(err)
		}

		fmt.Println(obj.name + ":")
		fmt.Println(string(data))
	}
}
