// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
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
	"github.com/awslabs/LogParserForKarpenter/s3"

	"k8s.io/client-go/util/homedir"
)

func main() {
	var filename string = "STDIN"
	var nodeclaimmap *map[string]lp4k.Nodeclaimstruct
	// helper map of k8snodename to nodeclaim
	var k8snodenamemap *map[string]string

	startTime := flag.String("start", "", "filter: only parse log lines at or after this timestamp (ISO 8601, e.g. 2026-06-18T12:00)")
	endTime := flag.String("end", "", "filter: only parse log lines at or before this timestamp (ISO 8601, e.g. 2026-06-18T14:00)")
	contextDefault := os.Getenv("LP4K_K8S_CONTEXT")
	k8sContext := flag.String("context", contextDefault, "Kubernetes context to use (overrides current-context, env: LP4K_K8S_CONTEXT)")
	var kubeconfig *string
	kubeconfigDefault := os.Getenv("KUBECONFIG")
	if kubeconfigDefault == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigDefault = filepath.Join(home, ".kube", "config")
		}
	}
	kubeconfig = flag.String("kubeconfig", kubeconfigDefault, "(optional) absolute path to the kubeconfig file (env: KUBECONFIG)")
	flag.Parse()

	// Set time range filter
	lp4k.FilterStart = *startTime
	lp4k.FilterEnd = *endTime

	// intialize maps
	nodeclaimes := make(map[string]lp4k.Nodeclaimstruct)
	nodeclaimmap = &nodeclaimes
	k8snodenames := make(map[string]string)
	k8snodenamemap = &k8snodenames
	reconcileIDs := make(map[string][]string)
	reconcileIDmap := &reconcileIDs

	// if we only have CMD itself i.e. no file args, we assume we get piped input and we check for STDIN
	if flag.NArg() == 0 {
		if termutil.Isatty(os.Stdin.Fd()) {
			fmt.Fprintf(os.Stderr, "Nothing on STDIN - trying to connect to kube-apiserver\n\n")

			ctx, clientSet := k8s.ConnectToK8s(kubeconfig, *k8sContext)

			// collect and parse logs
			k8s.CollectKarpenterLogs(ctx, clientSet, nodeclaimmap, k8snodenamemap, reconcileIDmap)
		} else {
			fmt.Fprintf(os.Stderr, "Attached to STDIN - parsing input until EOF or Ctrl-C\n")
			time.Sleep(1 * time.Second)

			signal.Notify(make(chan os.Signal, 1), os.Interrupt, syscall.SIGTERM)
			lp4k.ParseInput(os.Stdin, nodeclaimmap, k8snodenamemap, reconcileIDmap, filename)

			// STDIN empty or Ctrl-C
			fmt.Fprintf(os.Stderr, "Finished parsing STDIN\n\n")

			// print time range and nodeclaim output to STDOUT
			if minT, maxT := lp4k.TimeRange(nodeclaimmap); minT != "" {
				fmt.Fprintf(os.Stderr, "Time range: %s to %s\n\n", minT, maxT)
			}
			lp4k.PrintSortedResult(nodeclaimmap)

			// upload to S3 if configured
			if s3.IsEnabled() {
				if err := s3.UploadToS3(nodeclaimmap); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to upload to S3: %v\n", err)
				}
			}
		}
	} else {
		for _, arg := range flag.Args() {
			filename = arg

			fmt.Fprintf(os.Stderr, "Parsing input file %s\n", filename)

			file, err := os.Open(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			lp4k.ParseInput(file, nodeclaimmap, k8snodenamemap, reconcileIDmap, filename)

			fmt.Fprintf(os.Stderr, "Finished parsing input file %s\n\n", filename)
		}
		// print time range and nodeclaim output to STDOUT
		if minT, maxT := lp4k.TimeRange(nodeclaimmap); minT != "" {
			fmt.Fprintf(os.Stderr, "Time range: %s to %s\n\n", minT, maxT)
		}
		lp4k.PrintSortedResult(nodeclaimmap)

		// upload to S3 if configured
		if s3.IsEnabled() {
			if err := s3.UploadToS3(nodeclaimmap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to upload to S3: %v\n", err)
			}
		}
	}
}

