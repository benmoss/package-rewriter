package main

import (
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
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

	// Demonstrate JSON marshaling (common use case)
	data, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println("Application Spec:")
	fmt.Println(string(data))
}
