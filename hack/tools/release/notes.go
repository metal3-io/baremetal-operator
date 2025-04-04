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
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

/*
This tool prints all the titles of all PRs from previous release to HEAD.
This needs to be run *before* a tag is created.

Use these as the base of your release notes.
*/

const (
	features        = ":sparkles: New Features"
	bugs            = ":bug: Bug Fixes"
	documentation   = ":book: Documentation"
	warning         = ":warning: Breaking Changes"
	other           = ":seedling: Others"
	unknown         = ":question: Sort these by hand"
	superseded      = ":recycle: Superseded or Reverted"
	repoOwner       = "metal3-io"
	repoName        = "baremetal-operator"
	warningTemplate = ":rotating_light: This is a %s. Use it only for testing purposes.\nIf you find any bugs, file an [issue](https://github.com/metal3-io/baremetal-operator/issues/new/).\n\n"
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
	toTag = flag.String("releaseTag", "", "The tag or commit to end to.")
)

func main() {
	flag.Parse()
	os.Exit(run())
}

func latestTag() (string, error) {
	if toTag != nil && *toTag != "" {
		return *toTag, nil
	}
	return "", errors.New("RELEASE_TAG is not set")
}

// lastTag returns the tag to start collecting commits from based on the latestTag.
// For pre-releases and minor releases, it returns the latest minor release tag
// (e.g., for v1.9.0, v1.9.0-beta.0, or v1.9.0-rc.0, it returns v1.8.0).
// For patch releases, it returns the latest patch release tag (e.g., for v1.9.1 it returns v1.9.0).
func lastTag(latestTag string) (string, error) {
	if isBeta(latestTag) || isRC(latestTag) || isMinor(latestTag) {
		if index := strings.LastIndex(latestTag, "-"); index != -1 {
			latestTag = latestTag[:index]
		}
		latestTag = strings.TrimPrefix(latestTag, "v")

		semVersion, err := semver.New(latestTag)
		if err != nil {
			return "", errors.Wrapf(err, "parsing semver for %s", latestTag)
		}
		semVersion.Minor--
		lastReleaseTag := fmt.Sprintf("v%s", semVersion.String())
		return lastReleaseTag, nil
	}

	latestTag = strings.TrimPrefix(latestTag, "v")

	semVersion, err := semver.New(latestTag)
	if err != nil {
		return "", errors.Wrapf(err, "parsing semver for %s", latestTag)
	}
	semVersion.Patch--
	lastReleaseTag := fmt.Sprintf("v%s", semVersion.String())
	return lastReleaseTag, nil
}

func isBeta(tag string) bool {
	return strings.Contains(tag, "-beta.")
}

func isRC(tag string) bool {
	return strings.Contains(tag, "-rc.")
}

func isMinor(tag string) bool {
	return strings.HasSuffix(tag, ".0")
}

func run() int {
	latestTag, err := latestTag()
	if err != nil {
		log.Fatalf("Failed to get latestTag: %v", err)
	}
	lastTag, err := lastTag(latestTag)
	if err != nil {
		log.Fatalf("Failed to get lastTag: %v", err)
	}

	commitHash, err := getCommitHashFromNewTag(latestTag)
	if err != nil {
		log.Fatalf("Failed to get commit hash from latestTag %s: %v", latestTag, err)
	}

	cmd := exec.Command("git", "rev-list", lastTag+".."+commitHash, "--merges", "--pretty=format:%B") // #nosec G204:gosec

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
		case strings.HasPrefix(body, ":rocket:"), strings.HasPrefix(body, "🚀"):
			continue
		default:
			key = unknown
		}

		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		body = fmt.Sprintf("- %s", body)

		if strings.HasPrefix(c.merge, "Merge pull request") {
			_, err = fmt.Sscanf(c.merge, "Merge pull request %s from %s", &prNumber, &fork)
			if err != nil {
				log.Fatalf("Error parsing merge commit message: %v", err)
			}
		} else if strings.HasPrefix(c.merge, "Merge commit from fork") {
			_, err = fmt.Sscanf(c.merge, "Merge commit from fork")
			if err != nil {
				log.Fatalf("Error parsing merge commit message: %v", err)
			}
		} else {
			log.Fatalf("Unexpected merge commit message format: %s", c.merge)
		}

		merges[key] = append(merges[key], formatMerge(body, prNumber))
	}

	// Add empty superseded section, if not beta/rc, we don't cleanup those notes
	if !isBeta(latestTag) && !isRC(latestTag) {
		merges[superseded] = append(merges[superseded], "- `<insert superseded bumps and reverts here>`")
	}

	fmt.Println("<!-- markdownlint-disable no-inline-html line-length -->")
	if isBeta(latestTag) {
		fmt.Printf(warningTemplate, "BETA RELEASE")
	}
	if isRC(latestTag) {
		fmt.Printf(warningTemplate, "RELEASE CANDIDATE")
	}
	// TODO Turn this into a link (requires knowing the project name + organization)
	fmt.Printf("# Changes since %v\n\n", lastTag)

	// print the changes by category
	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) > 0 {
			fmt.Printf("## %v\n\n", key)
			for _, merge := range mergeslice {
				fmt.Println(merge)
			}
			fmt.Println()
		}

		// if we're doing beta/rc, print breaking changes and hide the rest of the changes
		if key == warning {
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

// getCommitHashFromNewTag returns the latest commit hash for the specified tag.
// For minor and pre releases, it returns the main branch's latest commit.
// For patch releases, it returns the latest commit on the corresponding release branch.
func getCommitHashFromNewTag(newTag string) (string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", errors.New("GITHUB_TOKEN is required")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	branch := "main"
	if !isBeta(newTag) {
		branch = getReleaseBranchFromTag(newTag)
		// Check if branch exist in upstream or not
		_, _, err := client.Repositories.GetBranch(ctx, repoOwner, repoName, branch)
		if err != nil {
			// If branch does not exist, defaults to main
			branch = "main"
		}
	}

	ref, _, err := client.Git.GetRef(ctx, repoOwner, repoName, "refs/heads/"+branch)
	if err != nil {
		log.Fatalf("Error fetching ref: %v", err)
	}
	commitHash := ref.GetObject().GetSHA()
	return commitHash, nil
}

func trimPrereleasePrefix(version string) string {
	if idx := strings.Index(version, "-"); idx != -1 {
		return version[:idx]
	}
	return version
}

func getReleaseBranchFromTag(tag string) string {
	tag = strings.TrimPrefix(tag, "v")
	tag = trimPrereleasePrefix(tag)
	if index := strings.LastIndex(tag, "."); index != -1 {
		tag = tag[:index]
	}
	return fmt.Sprintf("release-%s", tag)
}
