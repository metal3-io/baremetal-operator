// $GOPATH/src/kustomize-plugin-demo/main.go
package main

import (
	"fmt"
	"os"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type ValueAnnotator struct {
	Values []string `yaml:"values" json:"values"`
}

func main() {
	config := new(ValueAnnotator)
	fn := func(items []*yaml.RNode) ([]*yaml.RNode, error) {
		fmt.Fprintf(os.Stderr, "Loaded namespaces: %v\n", config.Values)
		var removeIndices []int
		for i := range items {

			if items[i].GetKind() == "Role" && items[i].GetName() == "baremetal-operator-manager-role" {
				updatedRoles, err := duplicateResourceOverNamespaces(items[i], config)
				removeIndices = append(removeIndices, i)
				items = append(items, updatedRoles...)
				if err != nil {
					return nil, err
				}
			}

			if items[i].GetKind() == "RoleBinding" && items[i].GetName() == "baremetal-operator-manager-rolebinding" {
				updatedRoles, err := duplicateResourceOverNamespaces(items[i], config)
				removeIndices = append(removeIndices, i)
				items = append(items, updatedRoles...)
				if err != nil {
					return nil, err
				}
			}

			items[i].GetDataMap()

		}

		// Remove the originally generated roles to not have duplicates, alternativley these could also be modified
		for i := len(removeIndices); i > 0; i-- {
			if len(removeIndices) > 0 {
				indexToRemove := removeIndices[i-1]
				items = append(items[:indexToRemove], items[indexToRemove+1:]...)
			}
		}

		// Ensure all namespaces are defined, as roles cannot be applied without existing namespaces
		for _, namespace := range config.Values {
			namespaceYaml := createNamespaceDefinition(namespace)
			items = append(items, namespaceYaml)
		}

		return items, nil
	}
	p := framework.SimpleProcessor{Config: config, Filter: kio.FilterFunc(fn)}
	cmd := command.Build(p, command.StandaloneDisabled, false)
	command.AddGenerateDockerfile(cmd)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func duplicateResourceOverNamespaces(roleConfig *yaml.RNode, config *ValueAnnotator) ([]*yaml.RNode, error) {
	var new_roles []*yaml.RNode
	for _, value := range config.Values {
		testNode := roleConfig.Copy()
		err := testNode.PipeE(yaml.SetK8sNamespace(value))
		if err != nil {
			return nil, err
		}
		new_roles = append(new_roles, testNode)
	}
	return new_roles, nil
}

func createNamespaceDefinition(namespace string) *yaml.RNode {
	namespaceNode := yaml.NewRNode(&yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "apiVersion"},
			{Kind: yaml.ScalarNode, Value: "v1"},
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.ScalarNode, Value: "Namespace"},
			{Kind: yaml.ScalarNode, Value: "metadata"},
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "name"},
					{Kind: yaml.ScalarNode, Value: namespace},
				},
			},
		},
	})
	return namespaceNode
}
