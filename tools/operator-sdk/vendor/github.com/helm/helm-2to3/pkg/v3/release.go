/*
Copyright The Helm Authors.

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

package v3

import (
	"fmt"
	"strings"
	stdtime "time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/time"

	v2chrtutil "k8s.io/helm/pkg/chartutil"
	v2chart "k8s.io/helm/pkg/proto/hapi/chart"
	v2rls "k8s.io/helm/pkg/proto/hapi/release"

	common "github.com/helm/helm-2to3/pkg/common"
)

// CreateRelease create a v3 release object from v3 release object
func CreateRelease(v2Rel *v2rls.Release) (*release.Release, error) {
	if v2Rel.Chart == nil || v2Rel.Info == nil {
		return nil, fmt.Errorf("No v2 chart or info metadata")
	}
	chrt, err := mapv2ChartTov3Chart(v2Rel.Chart)
	if err != nil {
		return nil, err
	}
	config, err := mapConfig(v2Rel.Config)
	if err != nil {
		return nil, err
	}
	first, err := mapTimestampToTime(v2Rel.Info.FirstDeployed)
	if err != nil {
		return nil, err
	}
	last, err := mapTimestampToTime(v2Rel.Info.LastDeployed)
	if err != nil {
		return nil, err
	}
	deleted, err := mapTimestampToTime(v2Rel.Info.Deleted)
	if err != nil {
		return nil, err
	}
	v3StatusStr, err := mapStatus(v2Rel.Info)
	if err != nil {
		return nil, err
	}

	hooks, err := mapHooks(v2Rel.Hooks, v2Rel.Info.Status.LastTestSuiteRun)
	if err != nil {
		return nil, err
	}

	return &release.Release{
		Name:      v2Rel.Name,
		Namespace: v2Rel.Namespace,
		Chart:     chrt,
		Config:    config,
		Info: &release.Info{
			FirstDeployed: first,
			LastDeployed:  last,
			Description:   v2Rel.Info.Description,
			Deleted:       deleted,
			Status:        release.Status(v3StatusStr),
			Notes:         v2Rel.Info.Status.Notes,
		},
		Manifest: v2Rel.Manifest,
		Hooks:    hooks,
		Version:  int(v2Rel.Version),
	}, nil
}

// StoreRelease stores a release object in Helm v3 storage
func StoreRelease(rel *release.Release, kubeConfig common.KubeConfig) error {
	cfg, err := GetActionConfig(rel.Namespace, kubeConfig)
	if err != nil {
		return err
	}

	return cfg.Releases.Create(rel)
}

func mapv2ChartTov3Chart(v2Chrt *v2chart.Chart) (*chart.Chart, error) {
	v3Chrt := new(chart.Chart)
	v3Chrt.Metadata = mapMetadata(v2Chrt)
	v3Chrt.Templates = mapTemplates(v2Chrt.Templates)
	err := mapDependencies(v2Chrt.Dependencies, v3Chrt)
	if err != nil {
		return nil, err
	}
	if v3Chrt.Values, err = mapConfig(v2Chrt.Values); err != nil {
		return nil, err
	}
	v3Chrt.Files = mapFiles(v2Chrt.Files)
	// Schema is set to nil as Schema wass introduced in Helm v3
	v3Chrt.Schema = nil
	// Lock is set to nil because v2 does not save the requirements lock file details
	// v3 should gather data as need be from the chart dependencies
	v3Chrt.Lock = nil
	return v3Chrt, nil
}

func mapMetadata(v2Chrt *v2chart.Chart) *chart.Metadata {
	if v2Chrt.Metadata == nil {
		return nil
	}
	metadata := new(chart.Metadata)
	metadata.Name = v2Chrt.Metadata.Name
	metadata.Home = v2Chrt.Metadata.Home
	metadata.Sources = v2Chrt.Metadata.Sources
	metadata.Version = v2Chrt.Metadata.Version
	metadata.Description = v2Chrt.Metadata.Description
	metadata.Keywords = v2Chrt.Metadata.Keywords
	metadata.Maintainers = mapMaintainers(v2Chrt.Metadata.Maintainers)
	metadata.Icon = v2Chrt.Metadata.Icon
	metadata.APIVersion = v2Chrt.Metadata.ApiVersion
	metadata.Condition = v2Chrt.Metadata.Condition
	metadata.Tags = v2Chrt.Metadata.Tags
	metadata.AppVersion = v2Chrt.Metadata.AppVersion
	metadata.Deprecated = v2Chrt.Metadata.Deprecated
	metadata.Annotations = v2Chrt.Metadata.Annotations
	metadata.KubeVersion = v2Chrt.Metadata.KubeVersion
	// v2 does not save the dependency metadata from requirements, so setting to nil
	// v3 should gather data as need be from the chart dependencies
	metadata.Dependencies = nil
	//Default to application
	metadata.Type = "application"
	return metadata
}

func mapMaintainers(v2Maintainers []*v2chart.Maintainer) []*chart.Maintainer {
	if v2Maintainers == nil {
		return nil
	}
	maintainers := []*chart.Maintainer{}
	for _, val := range v2Maintainers {
		maintainer := new(chart.Maintainer)
		maintainer.Name = val.Name
		maintainer.Email = val.Email
		maintainer.URL = val.Url
		maintainers = append(maintainers, maintainer)
	}
	return maintainers
}

func mapTemplates(v2Templates []*v2chart.Template) []*chart.File {
	if v2Templates == nil {
		return nil
	}
	files := []*chart.File{}
	for _, val := range v2Templates {
		file := new(chart.File)
		file.Name = val.Name
		file.Data = val.Data
		files = append(files, file)
	}
	return files
}

func mapDependencies(v2Dependencies []*v2chart.Chart, chart *chart.Chart) error {
	if v2Dependencies == nil {
		return nil
	}
	for _, val := range v2Dependencies {
		dependency, err := mapv2ChartTov3Chart(val)
		if err != nil {
			return err
		}
		chart.AddDependency(dependency)
	}
	return nil
}

func mapConfig(v2Config *v2chart.Config) (map[string]interface{}, error) {
	if v2Config == nil {
		return nil, nil
	}
	values, err := v2chrtutil.ReadValues([]byte(v2Config.Raw))
	if err != nil {
		return nil, err
	}
	return values, nil
}

func mapFiles(v2Files []*any.Any) []*chart.File {
	if mapFiles == nil {
		return nil
	}
	files := []*chart.File{}
	for _, f := range v2Files {
		file := new(chart.File)
		file.Name = f.TypeUrl
		file.Data = f.Value
		files = append(files, file)
	}
	return files
}

func mapStatus(v2Info *v2rls.Info) (string, error) {
	v2StatusStr, ok := v2rls.Status_Code_name[int32(v2Info.Status.Code)]
	if !ok {
		return "", fmt.Errorf("Failed to get v2 status")
	}
	// map to v3 status
	lowerCaseStr := strings.ToLower(v2StatusStr)
	v3StatusStr := strings.ReplaceAll(lowerCaseStr, "_", "-")
	if v3StatusStr == "deleted" {
		v3StatusStr = "uninstalled"
	}
	if v3StatusStr == "deleting" {
		v3StatusStr = "uninstalling"
	}
	return v3StatusStr, nil
}

func mapHooks(v2Hooks []*v2rls.Hook, v2LastTestSuiteRun *v2rls.TestSuite) ([]*release.Hook, error) {
	if v2Hooks == nil {
		return nil, nil
	}
	hooks := []*release.Hook{}
	for _, val := range v2Hooks {
		hook := new(release.Hook)
		hook.Name = val.Name
		hook.Kind = val.Kind
		hook.Path = val.Path
		hook.Manifest = val.Manifest
		events, err := mapHookEvents(val.Events)
		if err != nil {
			return nil, err
		}
		hook.Events = events
		hook.Weight = int(val.Weight)
		if err != nil {
			return nil, err
		}
		policies, err := mapHookDeletePolicies(val.DeletePolicies)
		if err != nil {
			return nil, err
		}
		hook.DeletePolicies = policies
		var lastRun *release.HookExecution
		lastRun, err = mapTestSuiteToHookExecution(hook.Name, v2LastTestSuiteRun)
		if err != nil {
			return nil, err
		}
		if lastRun != nil {
			hook.LastRun = *lastRun
		}
		hooks = append(hooks, hook)
	}
	return hooks, nil
}

func mapHookEvents(v2HookEvents []v2rls.Hook_Event) ([]release.HookEvent, error) {
	if v2HookEvents == nil {
		return nil, nil
	}
	hookEvents := []release.HookEvent{}
	for _, val := range v2HookEvents {
		v2EventStr, ok := v2rls.Hook_Event_name[int32(val)]
		if !ok {
			return nil, fmt.Errorf("Failed to get the v2 hook event string")
		}

		// map to v3 hook event
		lowerCaseStr := strings.ToLower(v2EventStr)
		v3EventStr := strings.ReplaceAll(lowerCaseStr, "_", "-")
		if strings.Contains(v3EventStr, "release-test") {
			v3EventStr = "test"
		}

		event := release.HookEvent(v3EventStr)
		hookEvents = append(hookEvents, event)
	}
	return hookEvents, nil
}

func mapHookDeletePolicies(v2HookDelPolicies []v2rls.Hook_DeletePolicy) ([]release.HookDeletePolicy, error) {
	if v2HookDelPolicies == nil {
		return nil, nil
	}
	hookDelPolicies := []release.HookDeletePolicy{}
	for _, val := range v2HookDelPolicies {
		v2PolicyStr, ok := v2rls.Hook_DeletePolicy_name[int32(val)]
		if !ok {
			return nil, fmt.Errorf("Failed to get the v2 hook delete policy")
		}

		// map to v3 hook delete policy
		lowerCaseStr := strings.ToLower(v2PolicyStr)
		v3PolicyStr := strings.ReplaceAll(lowerCaseStr, "_", "-")
		if !strings.Contains(v3PolicyStr, "before-hook-creation") {
			v3PolicyStr = "hook-" + v3PolicyStr
		}

		policy := release.HookDeletePolicy(strings.ToLower(v3PolicyStr))
		hookDelPolicies = append(hookDelPolicies, policy)
	}
	return hookDelPolicies, nil
}

func mapTimestampToTime(ts *timestamp.Timestamp) (time.Time, error) {
	var mappedTime stdtime.Time
	var err error
	if ts != nil {
		mappedTime, err = ptypes.Timestamp(ts)
		if err != nil {
			return time.Time{Time: mappedTime}, err
		}
	}
	return time.Time{Time: mappedTime}, nil
}

func mapTestSuiteToHookExecution(hookName string, testSuite *v2rls.TestSuite) (*release.HookExecution, error) {
	if testSuite == nil {
		return nil, nil
	}

	testSuiteResLen := len(testSuite.Results)

	if testSuiteResLen <= 0 {
		return nil, nil
	}

	for i := 0; i < testSuiteResLen; i++ {
		testRun := testSuite.Results[i]
		if testRun.Name != hookName {
			continue
		}
		hookEx := new(release.HookExecution)
		var err error
		hookEx.StartedAt, err = mapTimestampToTime(testRun.StartedAt)
		if err != nil {
			return nil, err
		}
		hookEx.CompletedAt, err = mapTimestampToTime(testRun.CompletedAt)
		if err != nil {
			return nil, err
		}
		v2RunStatusStr, ok := v2rls.TestRun_Status_name[int32(testRun.Status)]
		if !ok {
			return nil, fmt.Errorf("Failed to get the v2 test run status")
		}

		// Map to v3 test run status
		v3RunStatusStr := strings.ToLower(v2RunStatusStr)
		if v3RunStatusStr == "success" {
			v3RunStatusStr = "Succeeded"
		}
		if v3RunStatusStr == "failure" {
			v3RunStatusStr = "Failed"
		}

		hookEx.Phase = release.HookPhase(strings.Title(v3RunStatusStr))
		return hookEx, nil
	}

	return nil, nil
}
