package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	metal3clientset "github.com/metal3-io/baremetal-operator/pkg/clientset/versioned"
)

func main() {
	var kubeconfig *string
	var namespace *string
	var wipeAnnotation *bool

	kubeconfig = flag.String("kubeconfig", defaultKubeConfig(), "absolute path to the kubeconfig file")
	namespace = flag.String("n", "", "namespace for resources")
	wipeAnnotation = flag.Bool("a", false, "wipe the status annotation")
	flag.Parse()

	if *namespace == "" {
		fmt.Fprint(os.Stderr, "-n namespace is required\n")
		os.Exit(1)
	} else {
		fmt.Printf("namespace %q\n", *namespace)
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not build config: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("loaded kubeconfig from %s\n", *kubeconfig)

	client, err := metal3clientset.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not build client: %s\n", err.Error())
		os.Exit(1)
	}
	hostClient := client.Metal3V1alpha1().BareMetalHosts(*namespace)

	for _, hostName := range flag.Args() {
		fmt.Printf("starting %q ", hostName)

		getOpts := v1.GetOptions{}
		host, err := hostClient.Get(hostName, getOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not get host %q: %s\n", hostName, err)
			continue
		}

		if *wipeAnnotation {
			annotations := host.GetAnnotations()
			if annotations != nil {
				if _, ok := annotations[metal3v1alpha1.StatusAnnotation]; ok {
					delete(annotations, metal3v1alpha1.StatusAnnotation)
				}

				fmt.Printf("annotation... ")

				newHost, err := hostClient.Update(host)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to update host %q: %s\n", hostName, err)
					continue
				}
				host = newHost
			}
		}

		fmt.Printf("status... ")

		// wipe the existing status out
		host.Status = metal3v1alpha1.BareMetalHostStatus{}

		if _, err = hostClient.UpdateStatus(host); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to update host %q: %s\n", hostName, err)
			continue
		}

		fmt.Printf("done\n")
	}
}

func defaultKubeConfig() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}
	return ""
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
