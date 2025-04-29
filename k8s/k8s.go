// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package k8s

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	namespaceEnv         = "LP4K_KARPENTER_NAMESPACE"
	labelEnv             = "LP4K_KARPENTER_LABEL"
	updateEnv            = "LP4K_CM_UPDATE_FREQ"
	configmapEnv         = "LP4K_CM_PREFIX"
	configmapoverrideEnv = "LP4K_CM_OVERRIDE"
	nodeclaimprintEnv    = " LP4K_NODECLAIM_PRINT"
)

var namespace, label, configmappref, configmap string
var cmupdfreq time.Duration
var cmoverride, nodeclaimprint bool

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
			fmt.Fprintf(os.Stderr, "Invalid environment variable CM_UPDATE_FREQ, must be a valid time.Duration format like \"30s\" or \"2m10s\"\n")
			os.Exit(1)
		}
	}
	configmappref = os.Getenv(configmapEnv)
	if configmappref == "" {
		configmappref = "lp4k-cm"
	}
	cmoverridestr := os.Getenv(configmapoverrideEnv)
	if cmoverridestr == "" {
		cmoverride = false
	} else {
		cmoverride, err = strconv.ParseBool(cmoverridestr)
		if err != nil {
			cmoverride = false
		}
	}
	nodeclaimprintstr := os.Getenv(nodeclaimprintEnv)
	if nodeclaimprintstr == "" {
		nodeclaimprint = true
	} else {
		nodeclaimprint, err = strconv.ParseBool(nodeclaimprintstr)
		if err != nil {
			nodeclaimprint = true
		}
	}
	/*
		// move this out because otherwise it will get called every time from main.go
		if cmoverride {
			// use unique ConfigMap name and override on every start
			configmap = configmappref
			fmt.Fprintf(os.Stderr, "Using pods from namespace \"%s\" with label \"%s\"\n", namespace, label)
			fmt.Fprintf(os.Stderr, "Creating/overriding unique ConfigMap \"%s\" in namespace \"%s\" with updates every %s\n\n", configmap, namespace, cmupdfreq.String())

		} else {
			// construct ConfigMap name from time stamp - it probably contains non-DNS characters, so modify accordingly
			configmap = fmt.Sprintf("%s-%s", configmappref, strings.Replace(time.Now().Format(time.RFC3339), ":", "", -1))
			configmap, _, _ = strings.Cut(configmap, "+")
			configmap = strings.Replace(configmap, "T", "-", -1)
			fmt.Fprintf(os.Stderr, "Using pods from namespace \"%s\" with label \"%s\"\n", namespace, label)
			fmt.Fprintf(os.Stderr, "Creating ConfigMap \"%s\" in namespace \"%s\" with updates every %s\n\n", configmap, namespace, cmupdfreq.String())
		}
	*/
}

func ConnectToK8s(kubeconfig *string) (context.Context, *kubernetes.Clientset) {
	// use the current context in kubeconfig
	ctx := context.TODO()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build config from flags - %s\n", err.Error())
		os.Exit(1)
	}

	// create the K8s clientset
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create clientset from the given config - %s\n", err.Error())
		os.Exit(1)
	} else {
		fmt.Fprintf(os.Stderr, "Connected to K8s cluster\n")
	}

	return ctx, clientSet
}

// function to read nodeclaims from existing ConfigMap, required by tool lp4kcm as well!
func ReadnodeclaimsConfigMap(ctx context.Context, clientSet *kubernetes.Clientset, configmap string, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) {
	// use unique ConfigMap name and override on every start
	//configmap = configmappref

	fmt.Fprintf(os.Stderr, "\nRead existing ConfigMap \"%s\" in namespace \"%s\"\n", configmap, namespace)

	//clientSet.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})
	cm, err := clientSet.CoreV1().ConfigMaps(namespace).Get(ctx, configmap, metav1.GetOptions{})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get ConfigMap \"%s\" in namespace \"%s\" - %s\n", configmap, namespace, err.Error())
		os.Exit(1)
	}

	// populate nodeclaimmap from ConfigMap data
	lp4k.Populatenodeclaimmap(nodeclaimmap, cm.Data)
}

// internal function to create and write ConfigMap with nodeclaims
func nodeclaimsConfigMap(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) {
	// print current results every cmupdfreq seconds
	// create ConfigMap in same namespace like Karpenter namespace
	// ConfigMap data has to be map[string]string

	if cmoverride {
		// use unique ConfigMap name and override on every start
		configmap = configmappref
	} else {
		// construct ConfigMap name from time stamp - it probably contains non-DNS characters, so modify accordingly
		configmap = fmt.Sprintf("%s-%s", configmappref, strings.Replace(time.Now().Format(time.RFC3339), ":", "", -1))
		configmap, _, _ = strings.Cut(configmap, "+")
		configmap = strings.Replace(configmap, "T", "-", -1)
	}

	fmt.Fprintf(os.Stderr, "\nUsing ConfigMap \"%s\" in namespace \"%s\" with updates every %s\n", configmap, namespace, cmupdfreq.String())

	cm := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmap,
			Namespace: namespace,
		},
	}

	fmt.Fprintf(os.Stderr, "\nCreate empty ConfigMap \"%s\" in namespace \"%s\"\n", configmap, namespace)
	fmt.Fprintf(os.Stderr, "First nodeclaim data in ConfigMap \"%s/%s\" in %s (%.0f seconds), type Ctrl-C to end program\n", namespace, configmap, cmupdfreq.String(), cmupdfreq.Seconds())
	clientSet.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})

	// update nodeclaim ConfigMap every cmupdfreq seconds cmupdfreq
	//for range time.Tick(time.Second * 30) {
	for range time.Tick(cmupdfreq) {
		timeStr := fmt.Sprint(time.Now().Format(time.RFC850))

		// get actual data from nodeclaimmap
		cm.Data = lp4k.ConvertResult(nodeclaimmap)

		fmt.Fprintf(os.Stderr, "\nUpdate ConfigMap\n")
		clientSet.CoreV1().ConfigMaps(namespace).Update(ctx, &cm, metav1.UpdateOptions{})

		fmt.Fprintf(os.Stderr, "Current time: %s\n", timeStr)
		if nodeclaimprint {
			lp4k.PrintSortedResult(nodeclaimmap)
		}
		fmt.Fprintf(os.Stderr, "\nNext nodeclaim data in ConfigMap \"%s/%s\" in %s (%.0f seconds), type Ctrl-C to end program\n", namespace, configmap, cmupdfreq.String(), cmupdfreq.Seconds())
	}
}

func CollectKarpenterLogs(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct, k8snodenamemap *map[string]string) {
	// get the pods as ListItems
	fmt.Fprintf(os.Stderr, "\nRetrieving pods from namespace \"%s\" with label \"%s\"\n", namespace, label)
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to get pods\n")
		os.Exit(1)
	} else {
		if len(pods.Items) == 0 {
			fmt.Fprintf(os.Stderr, "\nEmpty pod list - no pods in namespace \"%s\" with label \"%s\" - finishing\n", namespace, label)
			os.Exit(1)
		} else {
			fmt.Fprintf(os.Stderr, "\nFound pods in namespace \"%s\" with label \"%s\"\n", namespace, label)
		}

	}
	// get the pod lists first, then get the podLogs from each of the pods
	// use channel for blocking reasons
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	podItems := pods.Items
	// for i := 0; i < len(podItems); i++ {
	for i := range podItems {
		fmt.Fprintf(os.Stderr, "Streaming logs from pod \"%s\" in namespace \"%s\"\n", podItems[i].Name, podItems[i].Namespace)
		podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(podItems[i].Name, &v1.PodLogOptions{
			Follow: true,
		}).Stream(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stream pod logs - %s\n", err.Error())
			os.Exit(1)
		}
		defer podLogs.Close()

		go lp4k.NonBlockingParser(bufio.NewScanner(podLogs), nodeclaimmap, k8snodenamemap, "STDIN", 0)
	}

	// read already existing ConfigMap in override mode only
	if cmoverride {
		ReadnodeclaimsConfigMap(ctx, clientSet, configmappref, nodeclaimmap)
	}

	// create and update ConfigMap with nodeclaims
	go nodeclaimsConfigMap(ctx, clientSet, nodeclaimmap)

	// required to block until Ctrl-C
	defer func() {
		<-ch
	}()
}
