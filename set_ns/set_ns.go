package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var currentDir string

func init() {
	var err error
	currentDir, err = os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("Error getting current directory: %v", err))
	}
}

var (
	annotatorPath    = filepath.Join(currentDir, "..", "config", "overlays", "namespaced", "roles-ns-annotator.yaml")
	managerPatchPath = filepath.Join(currentDir, "..", "config", "overlays", "namespaced", "namespaced-manager-patch.yaml")
)

func updateAnnotatorFile(namespacesToSet string) error {
	data, err := os.ReadFile(annotatorPath)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}

	namespaces := strings.Split(namespacesToSet, ",")
	for i := range namespaces {
		namespaces[i] = strings.TrimSpace(namespaces[i])
	}
	doc["values"] = namespaces

	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(annotatorPath, out, 0644)
}

func setWatchNamespaceEnvVar(namespacesToSet string) error {
	data, err := os.ReadFile(managerPatchPath)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}

	containers := doc["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	envList := container["env"].([]interface{})
	envVar := envList[0].(map[string]interface{})

	envVar["value"] = namespacesToSet

	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(managerPatchPath, out, 0644)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: ./app \"namespace1,namespace2\"")
		os.Exit(1)
	}

	namespacesToSet := os.Args[1]

	if err := updateAnnotatorFile(namespacesToSet); err != nil {
		fmt.Printf("Error updating annotator file: %v\n", err)
		os.Exit(1)
	}

	if err := setWatchNamespaceEnvVar(namespacesToSet); err != nil {
		fmt.Printf("Error updating manager patch file: %v\n", err)
		os.Exit(1)
	}
}
