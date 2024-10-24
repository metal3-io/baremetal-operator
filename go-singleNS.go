package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

func main() {
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	annotatorPath := strings.Join([]string{"namespaced", "roles-ns-annotator.yaml"}, "/")
	managerPatchPath := strings.Join([]string{"namespaced", "namespaced-manager-patch.yaml"}, "/")

	fmt.Println(currentDir, managerPatchPath, annotatorPath)

	file, err := os.Open(annotatorPath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Read the file
	byteValue, _ := io.ReadAll(file)

	// Create a map to hold the YAML data
	var data map[string]interface{}

	// Unmarshal the YAML data into the map
	err = yaml.Unmarshal(byteValue, &data)
	if err != nil {
		fmt.Println("Error unmarshalling YAML:", err)
		return
	}

	newNamespaces := []interface{}{"S2", "s3", "s4"}
	data["values"] = newNamespaces

	// Marshal the updated data back to YAML
	updatedYAML, err := yaml.Marshal(&data)
	if err != nil {
		fmt.Println("Error marshalling YAML:", err)
		return
	}

	// Write the updated YAML back to the file
	err = os.WriteFile(annotatorPath, updatedYAML, 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	fmt.Println(annotatorPath, " updated with new namespaces successfully!")

	targetString := "value:"
	newValue := "ns1,ns2,ns3"

	// Open the input file
	file, err = os.Open(managerPatchPath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	// Read file line by line and store in lines slice
	for scanner.Scan() {
		line := scanner.Text()

		// Check if the line contains 'value:'
		if strings.Contains(line, targetString) {
			// Split by "value:" and replace the second part with newValue
			parts := strings.Split(line, targetString)
			if len(parts) == 2 {
				line = parts[0] + targetString + " " + newValue
			}
		}

		// Add the modified or unmodified line to the slice
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Open the same file for writing and overwrite its content
	file, err = os.OpenFile(managerPatchPath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Error opening file for writing:", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// Write modified content back to the file
	for _, line := range lines {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	}

	writer.Flush()

	fmt.Println(managerPatchPath, " updated with new namespaces successfully!")
}
