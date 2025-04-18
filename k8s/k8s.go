// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package k8s

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
)

const (
	// set namespace and label
	namespace = "karpenter"
	configmap = "karpenter-nodeclaims-cm"
	label     = "app.kubernetes.io/name=karpenter"
)

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
		fmt.Fprintf(os.Stderr, "Connected to K8s cluster - parsing logs until Ctrl-C\n")
	}

	return ctx, clientSet
}

func NodeclaimsConfigMap(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) {
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
		Data: lp4k.ConvertResult(nodeclaimmap),
	}

	fmt.Println("Create ConfigMap")
	clientSet.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})

	// update nodeclaim ConfigMap every 30s
	for range time.Tick(time.Second * 30) {
		timeStr := fmt.Sprint(time.Now().Format(time.RFC850))

		// get actual data from nodeclaimmap
		cm.Data = lp4k.ConvertResult(nodeclaimmap)

		fmt.Println("Update ConfigMap")
		clientSet.CoreV1().ConfigMaps(namespace).Update(ctx, &cm, metav1.UpdateOptions{})

		fmt.Println("Current time: ", timeStr)
		lp4k.PrintSortedResult(nodeclaimmap)
		fmt.Println("Type Ctrl-C to end program")
	}
}

// helper function
func parseLogs(scanner *bufio.Scanner, ch chan bool, nodeclaimmap *map[string]lp4k.Nodeclaimstruct, k8snodenamemap *map[string]string) {
	//func writeLogs(reader *bufio.Scanner, file *os.File, ch chan bool) {
	defer func() {
		ch <- true
	}()
	for scanner.Scan() {
		logline := scanner.Text()
		lp4k.ParseKarpenterLogs(logline, nodeclaimmap, k8snodenamemap, "STDIN", 0)
	}
}

func CollectKarpenterLogs(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct, k8snodenamemap *map[string]string) error {
	// get the pods as ListItems
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		log.Println(err, "Failed to get pods")
		return err
	}
	// get the pod lists first
	// then get the podLogs from each of the pods
	// write to files concurrently
	// use channel for blocking reasons
	// ch := make(chan os.Signal, 1)
	// signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	ch := make(chan bool)
	podItems := pods.Items
	// for i := 0; i < len(podItems); i++ {
	for i := range podItems {
		podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(podItems[i].Name, &v1.PodLogOptions{
			Follow: true,
		}).Stream(ctx)
		if err != nil {
			return err
		}
		defer podLogs.Close()

		scanner := bufio.NewScanner(podLogs)

		go parseLogs(scanner, ch, nodeclaimmap, k8snodenamemap)
		//go writeLogs(reader, file, ch)
	}

	//required for sync with writeLogs
	<-ch

	return nil
}
