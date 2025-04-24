// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package k8s

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
)

const (
	// environment variables
	namespaceEnv = "KARPENTER_NAMESPACE"
	labelEnv     = "KARPENTER_LABEL"
	updateEnv    = "KARPENTER_CM_UPDATE_FREQ"
	// hard-coded
	configmap = "karpenter-nodeclaims-cm"
)

var namespace, label string
var cmupdfreq time.Duration

// internal helper function to determine Karpenter namespace and label via OS environment, if not set use defaults
func init() {
	var err error

	namespace = os.Getenv(namespaceEnv)
	if namespace == "" {
		namespace = "karpenter"
	}
	label = os.Getenv(labelEnv)
	if label == "" {
		label = "app.kubernetes.io/name=karpenter"
	}
	cmupdfreqstr := os.Getenv(updateEnv)
	if cmupdfreqstr == "" {
		cmupdfreqstr = "30s"
		cmupdfreq, _ = time.ParseDuration(cmupdfreqstr)
	} else {
		cmupdfreq, err = time.ParseDuration(cmupdfreqstr)
		if err != nil {
			fmt.Println("Invalid environment variable CM_UPDATE_FREQ, must be a valid time.Duration format like \"30s\" or \"2m10s\"")
			os.Exit(1)
		}
	}
}

func ConnectToK8s(kubeconfig *string) (context.Context, *kubernetes.Clientset) {
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
		fmt.Println("Connected to K8s clustern")
	}

	return ctx, clientSet
}

func NodeclaimsConfigMap(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) {
	// print current results every cmupdfreq seconds
	// create ConfigMap in same namespace like Karpenter namespace
	// ConfigMap data has to be map[string]string
	cm := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmap,
			Namespace: namespace,
		},
		Data: lp4k.ConvertResult(nodeclaimmap),
	}

	fmt.Println("Create ConfigMap")
	fmt.Printf("\nNext update in %s (%.0f seconds), type Ctrl-C to end program\n", cmupdfreq.String(), cmupdfreq.Seconds())
	clientSet.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})

	// update nodeclaim ConfigMap every cmupdfreq seconds cmupdfreq
	//for range time.Tick(time.Second * 30) {
	for range time.Tick(cmupdfreq) {
		timeStr := fmt.Sprint(time.Now().Format(time.RFC850))

		// get actual data from nodeclaimmap
		cm.Data = lp4k.ConvertResult(nodeclaimmap)

		fmt.Println("\nUpdate ConfigMap")
		clientSet.CoreV1().ConfigMaps(namespace).Update(ctx, &cm, metav1.UpdateOptions{})

		fmt.Println("Current time: ", timeStr)
		lp4k.PrintSortedResult(nodeclaimmap)
		fmt.Printf("\nNext update in %s (%.0f seconds), type Ctrl-C to end program\n", cmupdfreq.String(), cmupdfreq.Seconds())
	}
}

func CollectKarpenterLogs(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct, k8snodenamemap *map[string]string) {
	// get the pods as ListItems
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		log.Println(err, "Failed to get pods")
		os.Exit(1)
	} else {
		if len(pods.Items) == 0 {
			fmt.Printf("\nEmpty pod list - no pods in namespace \"%s\" with label \"%s\" - finishing\n", namespace, label)
			os.Exit(1)
		} else {
			fmt.Printf("\nFound pods in namespace \"%s\" with label \"%s\"\n", namespace, label)
		}

	}
	// get the pod lists first, then get the podLogs from each of the pods
	// use channel for blocking reasons
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	podItems := pods.Items
	// for i := 0; i < len(podItems); i++ {
	for i := range podItems {
		fmt.Printf("Streaming logs from pod \"%s\" in namespace \"%s\"\n", podItems[i].Name, podItems[i].Namespace)
		podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(podItems[i].Name, &v1.PodLogOptions{
			Follow: true,
		}).Stream(ctx)
		if err != nil {
			log.Println(err, "Failed to stream pod logs")
			os.Exit(1)
		}
		defer podLogs.Close()

		go lp4k.NonBlockingParser(bufio.NewScanner(podLogs), nodeclaimmap, k8snodenamemap, "STDIN", 0)
	}

	// required to block until Ctrl-C
	defer func() {
		<-ch
	}()
}
