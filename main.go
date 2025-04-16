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
	"github.com/youwalther65/KarpenterLogParser/k8s"
	klp "github.com/youwalther65/KarpenterLogParser/parser"

	"k8s.io/client-go/util/homedir"
)

func main() {
	var logline, filename string
	var nodeclaimmap *map[string]klp.Nodeclaimstruct
	// helper map of k8snodename to nodeclaim
	var k8snodenamemap *map[string]string
	var inputline int

	// intialize maps
	nodeclaimes := make(map[string]klp.Nodeclaimstruct)
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

			// print current results every 30s
			go k8s.NodeclaimsConfigMap(ctx, clientSet, nodeclaimmap)

			// collect logs
			err := k8s.CollectKarpenterLogs(ctx, clientSet, nodeclaimmap, k8snodenamemap)
			if err != nil {
				log.Println(err, "Failed to collect Karpenter logs")
			}
		} else {
			fmt.Fprintf(os.Stderr, "Attached to STDIN - parsing iput until EOF or Ctrl-C\n")
			time.Sleep(1 * time.Second)
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			scanner := bufio.NewScanner(os.Stdin)
			// this is used to reference line in STDIN
			inputline = 0
			for scanner.Scan() {
				//fmt.Println(scanner.Text())
				// main parsing logic
				logline = scanner.Text()
				klp.ParseKarpenterLogs(logline, nodeclaimmap, k8snodenamemap, "STDIN", inputline)
				// we wait until Ctrl-C because we have an input from something like "kubectl logs -n karpenter -l=app.kubernetes.io/name=karpenter -f"
				go func() {
					<-c
					if err := scanner.Err(); err != nil {
						log.Fatal(err)
					}
				}()
			}
			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
			// STDIN empty or Ctrl-C
			fmt.Fprintf(os.Stderr, "Finished parsing STDIN\n\n")
			// print nodeclaim output to STDOUT
			klp.PrintSortedResult(nodeclaimmap)
		}
	} else {
		for _, arg := range os.Args[1:] {
			filename = arg

			//fmt.Fprintf(os.Stderr, "STDERR: Info - parsing input file %s\n", filename)

			file, err := os.Open(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			// this is used to reference lines in input files
			inputline = 0

			// main parsing logic
			for scanner.Scan() {
				logline = scanner.Text()
				klp.ParseKarpenterLogs(logline, nodeclaimmap, k8snodenamemap, filename, inputline)
			}

			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
		}
		// print nodeclaim output to STDOUT
		klp.PrintSortedResult(nodeclaimmap)
	}
}

/*
func printNodeclaims(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]klp.Nodeclaimstruct) {
	// print current results every minute
	// create ConfigMap in same namespace
	// CM data
	// Create a map with string keys and values
	cm := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmap,
			Namespace: namespace,
		},
		Data: klp.ConvertResult(nodeclaimmap),
	}

	fmt.Println("Create ConfigMap")
	clientSet.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})

	// update nodeclaim ConfigMap every 30s
	for range time.Tick(time.Second * 30) {
		timeStr := fmt.Sprint(time.Now().Format(time.RFC850))

		// get actual data from nodeclaimmap
		cm.Data = klp.ConvertResult(nodeclaimmap)

		fmt.Println("Update ConfigMap")
		clientSet.CoreV1().ConfigMaps(namespace).Update(ctx, &cm, metav1.UpdateOptions{})

		fmt.Println("Current time: ", timeStr)
		klp.PrintSortedResult(nodeclaimmap)
		fmt.Println("Type Ctrl-C to end program")
	}
}

func connectToK8s(kubeconfig *string) (context.Context, *kubernetes.Clientset) {
	// use the current context in kubeconfig
	ctx := context.TODO()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Println(err, "Failed to build config from flags")
		os.Exit(1)
	}

	// create the K8s clientset
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Println("Failed to create clientset from the given config")
		os.Exit(1)
	} else {
		fmt.Fprintf(os.Stderr, "Connected to K8s cluster - parsing logs until Ctrl-C\n\n")
	}

	return ctx, clientSet
}
*/
