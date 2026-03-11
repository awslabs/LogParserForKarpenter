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
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
	"github.com/awslabs/LogParserForKarpenter/s3"
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
// handle ConfigMap override logic as well
func init() {
	var err error
	namespace = getEnvOrDefault(namespaceEnv, "kube-system")
	label = getEnvOrDefault(labelEnv, "app.kubernetes.io/name=karpenter")
	cmupdfreqstr := getEnvOrDefault(updateEnv, "30s")
	cmupdfreq, err = time.ParseDuration(cmupdfreqstr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid environment variable CM_UPDATE_FREQ, must be a valid time.Duration format like \"30s\" or \"2m10s\"\n")
		os.Exit(1)
	}
	configmappref = getEnvOrDefault(configmapEnv, "lp4k-cm")
	cmoverride = getEnvBool(configmapoverrideEnv, false)
	nodeclaimprint = getEnvBool(nodeclaimprintEnv, true)
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, _ := strconv.ParseBool(val)
	return b
}

func ConnectToK8s(kubeconfig *string) (context.Context, *kubernetes.Clientset) {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build config from flags - %s\n", err.Error())
		os.Exit(1)
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create clientset from the given config - %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Connected to K8s cluster\n")
	return context.Background(), clientSet
}

// function to read nodeclaims from existing ConfigMap, required by tool lp4kcm as well!
func ReadnodeclaimsConfigMap(ctx context.Context, clientSet *kubernetes.Clientset, configmap string, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) {
	// use unique ConfigMap name and override on every start
	fmt.Fprintf(os.Stderr, "\nRead existing ConfigMap \"%s\" in namespace \"%s\"\n", configmap, namespace)
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
		// construct ConfigMap name from time stamp
		configmap = fmt.Sprintf("%s-%s", configmappref, s3.GetStartTimestamp())
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
	// update nodeclaim ConfigMap every cmupdfreq seconds
	for range time.Tick(cmupdfreq) {
		// get actual data from nodeclaimmap
		cm.Data = lp4k.ConvertResult(nodeclaimmap)
		fmt.Fprintf(os.Stderr, "\nUpdate ConfigMap\n")
		clientSet.CoreV1().ConfigMaps(namespace).Update(ctx, &cm, metav1.UpdateOptions{})
		fmt.Fprintf(os.Stderr, "Current time: %s\n", time.Now().Format(time.RFC850))
		if nodeclaimprint {
			lp4k.PrintSortedResult(nodeclaimmap)
		}

		// upload to S3 if configured
		if s3.IsEnabled() {
			if err := s3.UploadToS3(nodeclaimmap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to upload to S3: %v\n", err)
			}
		}

		fmt.Fprintf(os.Stderr, "\nNext nodeclaim data in ConfigMap \"%s/%s\" in %s (%.0f seconds), type Ctrl-C to end program\n", namespace, configmap, cmupdfreq.String(), cmupdfreq.Seconds())
	}
}

func CollectKarpenterLogs(ctx context.Context, clientSet *kubernetes.Clientset, nodeclaimmap *map[string]lp4k.Nodeclaimstruct, k8snodenamemap *map[string]string) {
	// get the pods as ListItems
	fmt.Fprintf(os.Stderr, "\nRetrieving pods from namespace \"%s\" with label \"%s\"\n", namespace, label)
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get pods: %v\n", err)
		os.Exit(1)
	}
	if len(pods.Items) == 0 {
		fmt.Fprintf(os.Stderr, "\nEmpty pod list - no pods in namespace \"%s\" with label \"%s\" - finishing\n", namespace, label)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\nFound pods in namespace \"%s\" with label \"%s\"\n", namespace, label)
	// get the pod lists first, then get the podLogs from each of the pods
	// use channel for blocking reasons
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	for i := range pods.Items {
		fmt.Fprintf(os.Stderr, "Streaming logs from pod \"%s\" in namespace \"%s\"\n", pods.Items[i].Name, pods.Items[i].Namespace)
		podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(pods.Items[i].Name, &v1.PodLogOptions{Follow: true}).Stream(ctx)
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
	defer func() { <-ch }()
}