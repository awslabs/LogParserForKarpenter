// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/awslabs/LogParserForKarpenter/k8s"
	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
	"k8s.io/client-go/util/homedir"
)

func main() {
	//var logline, filename string
	var cmname string
	var nodeclaimmap *map[string]lp4k.Nodeclaimstruct

	// intialize maps
	nodeclaimes := make(map[string]lp4k.Nodeclaimstruct)
	nodeclaimmap = &nodeclaimes
	/*
		k8snodenames := make(map[string]string)
		k8snodenamemap = &k8snodenames
	*/

	// parse the .kubeconfig file
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	ctx, clientSet := k8s.ConnectToK8s(kubeconfig)

	for _, arg := range os.Args[1:] {
		cmname = arg

		fmt.Fprintf(os.Stderr, "\nParsing ConfigMap %s\n", cmname)

		// main parsing logic
		k8s.ReadnodeclaimsConfigMap(ctx, clientSet, cmname, nodeclaimmap)

		fmt.Fprintf(os.Stderr, "Finished parsing ConfigMap %s\n", cmname)
	}
	fmt.Fprintf(os.Stderr, "\n")
	// print nodeclaim output to STDOUT
	lp4k.PrintSortedResult(nodeclaimmap)
}
