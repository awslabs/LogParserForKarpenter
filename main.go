// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	termutil "github.com/andrew-d/go-termutil"
	"github.com/awslabs/LogParserForKarpenter/k8s"
	lp4k "github.com/awslabs/LogParserForKarpenter/parser"

	"k8s.io/client-go/util/homedir"
)

func main() {
	//var logline, filename string
	var filename string
	var nodeclaimmap *map[string]lp4k.Nodeclaimstruct
	// helper map of k8snodename to nodeclaim
	var k8snodenamemap *map[string]string
	//var inputline int

	// intialize maps
	nodeclaimes := make(map[string]lp4k.Nodeclaimstruct)
	nodeclaimmap = &nodeclaimes
	k8snodenames := make(map[string]string)
	k8snodenamemap = &k8snodenames

	// if we only have CMD itself i.e. len(os.Args) == 1 we assume we get piped input and we check for STDIN
	if len(os.Args) == 1 {
		if termutil.Isatty(os.Stdin.Fd()) {
			fmt.Fprintf(os.Stderr, "Nothing on STDIN - trying to connect to kube-apiserver\n\n")
			// parse the .kubeconfig file
			var kubeconfig *string
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
			} else {
				kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
			}
			flag.Parse()

			ctx, clientSet := k8s.ConnectToK8s(kubeconfig)

			// print current results every k8s.cmupdfreq seconds (currently hard-coded 30s)
			go k8s.NodeclaimsConfigMap(ctx, clientSet, nodeclaimmap)

			// collect logs
			k8s.CollectKarpenterLogs(ctx, clientSet, nodeclaimmap, k8snodenamemap)
		} else {
			fmt.Fprintf(os.Stderr, "Attached to STDIN - parsing iput until EOF or Ctrl-C\n")
			time.Sleep(1 * time.Second)

			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

			// main parsing logic
			lp4k.BlockingParser(ch, bufio.NewScanner(os.Stdin), nodeclaimmap, k8snodenamemap, filename, 0)

			// STDIN empty or Ctrl-C
			fmt.Fprintf(os.Stderr, "Finished parsing STDIN\n\n")

			// print nodeclaim output to STDOUT
			lp4k.PrintSortedResult(nodeclaimmap)
		}
	} else {
		for _, arg := range os.Args[1:] {
			filename = arg

			fmt.Fprintf(os.Stderr, "Parsing input file %s\n", filename)

			file, err := os.Open(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			// main parsing logic
			lp4k.NonBlockingParser(bufio.NewScanner(file), nodeclaimmap, k8snodenamemap, filename, 0)

			fmt.Fprintf(os.Stderr, "Finished parsing input file %s\n", filename)
		}
		// print nodeclaim output to STDOUT
		lp4k.PrintSortedResult(nodeclaimmap)
	}
}
