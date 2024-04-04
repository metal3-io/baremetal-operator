//go:build tools
// +build tools

/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

/*
This tool prints all the titles of all PRs from previous release to HEAD.
This needs to be run *before* a tag is created.

Use these as the base of your release notes.
*/

const (
	features      = ":sparkles: New Features"
	bugs          = ":bug: Bug Fixes"
	documentation = ":book: Documentation"
	warning       = ":warning: Breaking Changes"
	other         = ":seedling: Others"
	unknown       = ":question: Sort these by hand"
	superseded    = ":recycle: Superseded or Reverted"
)

const (
	warningTemplate = ":rotating_light: This is a %s. Use it only for testing purposes. If you find any bugs, file an [issue](https://github.com/metal3-io/baremetal-operator/issues/new/).\n\n"
)

var (
	outputOrder = []string{
		warning,
		features,
		bugs,
		documentation,
		other,
		unknown,
		superseded,
	}

	fromTag = flag.String("from", "", "The tag or commit to start from.")
)

func main() {
	flag.Parse()
	os.Exit(run())
}

func latestTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		return firstCommit()
	}
	return string(bytes.TrimSpace(out))
}

func lastTag() string {
	if fromTag != nil && *fromTag != "" {
		return *fromTag
	}
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		return firstCommit()
	}
	return string(bytes.TrimSpace(out))
}

func isBeta(tag string) bool {
	return strings.Contains(tag, "-beta.")
}

func isRC(tag string) bool {
	return strings.Contains(tag, "-rc.")
}

func firstCommit() string {
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "UNKNOWN"
	}
	return string(bytes.TrimSpace(out))
}

func run() int {
	lastTag := lastTag()
	latestTag := latestTag()
	cmd := exec.Command("git", "rev-list", lastTag+"..HEAD", "--merges", "--pretty=format:%B") // #nosec G204:gosec

	merges := map[string][]string{
		features:      {},
		bugs:          {},
		documentation: {},
		warning:       {},
		other:         {},
		unknown:       {},
		superseded:    {},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error")
		fmt.Println(string(out))
		return 1
	}

	commits := []*commit{}
	outLines := strings.Split(string(out), "\n")
	for _, line := range outLines {
		line = strings.TrimSpace(line)
		last := len(commits) - 1
		switch {
		case strings.HasPrefix(line, "commit"):
			commits = append(commits, &commit{})
		case strings.HasPrefix(line, "Merge"):
			commits[last].merge = line
			continue
		case line == "":
		default:
			commits[last].body = line
		}
	}

	for _, c := range commits {
		body := strings.TrimSpace(c.body)
		var key, prNumber, fork string
		switch {
		case strings.HasPrefix(body, ":sparkles:"), strings.HasPrefix(body, "✨"):
			key = features
			body = strings.TrimPrefix(body, ":sparkles:")
			body = strings.TrimPrefix(body, "✨")
		case strings.HasPrefix(body, ":bug:"), strings.HasPrefix(body, "🐛"):
			key = bugs
			body = strings.TrimPrefix(body, ":bug:")
			body = strings.TrimPrefix(body, "🐛")
		case strings.HasPrefix(body, ":book:"), strings.HasPrefix(body, "📖"):
			key = documentation
			body = strings.TrimPrefix(body, ":book:")
			body = strings.TrimPrefix(body, "📖")
		case strings.HasPrefix(body, ":seedling:"), strings.HasPrefix(body, "🌱"):
			key = other
			body = strings.TrimPrefix(body, ":seedling:")
			body = strings.TrimPrefix(body, "🌱")
		case strings.HasPrefix(body, ":running:"), strings.HasPrefix(body, "🏃"):
			// This has been deprecated in favor of :seedling:
			key = other
			body = strings.TrimPrefix(body, ":running:")
			body = strings.TrimPrefix(body, "🏃")
		case strings.HasPrefix(body, ":warning:"), strings.HasPrefix(body, "⚠️"):
			key = warning
			body = strings.TrimPrefix(body, ":warning:")
			body = strings.TrimPrefix(body, "⚠️")
		default:
			key = unknown
		}

		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		body = fmt.Sprintf("- %s", body)
		fmt.Sscanf(c.merge, "Merge pull request %s from %s", &prNumber, &fork)
		merges[key] = append(merges[key], formatMerge(body, prNumber))
	}

	// Add empty superseded section, if not beta/rc, we don't cleanup those notes
	if !isBeta(latestTag) && !isRC(latestTag) {
		merges[superseded] = append(merges[superseded], "- `<insert superseded bumps and reverts here>`")
	}

	// TODO Turn this into a link (requires knowing the project name + organization)
	fmt.Printf("Changes since %v\n---\n", lastTag)

	// print the changes by category
	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) > 0 {
			fmt.Println("## " + key)
			for _, merge := range mergeslice {
				fmt.Println(merge)
			}
			fmt.Println()
		}

		// if we're doing beta/rc, print breaking changes and hide the rest of the changes
		if key == warning {
			if isBeta(latestTag) {
				fmt.Printf(warningTemplate, "BETA RELEASE")
			}
			if isRC(latestTag) {
				fmt.Printf(warningTemplate, "RELEASE CANDIDATE")
			}
			if isBeta(latestTag) || isRC(latestTag) {
				fmt.Printf("<details>\n")
				fmt.Printf("<summary>More details about the release</summary>\n\n")
			}
		}
	}

	// then close the details if we had it open
	if isBeta(latestTag) || isRC(latestTag) {
		fmt.Printf("</details>\n\n")
	}

	fmt.Printf("The image for this release is: %v\n", latestTag)
	fmt.Println("\n_Thanks to all our contributors!_ 😊")

	return 0
}

type commit struct {
	merge string
	body  string
}

func formatMerge(line, prNumber string) string {
	if prNumber == "" {
		return line
	}
	return fmt.Sprintf("%s (%s)", line, prNumber)
}
