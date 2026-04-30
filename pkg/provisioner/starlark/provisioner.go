/*

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

// Provisioner interface methods; each delegates to a named Starlark function.

package starlark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/starlark/starlib"
	"go.starlark.net/starlark"
)

func (p *starlarkProvisioner) TryInit(ctx context.Context) (bool, error) {
	p.log.Info("starlark: try_init")

	m, err := p.CallExpectingDict(ctx, "try_init")
	if err != nil {
		return false, err
	}

	return starlib.MapField[bool](m, "ready"), nil
}

func (p *starlarkProvisioner) HasCapacity(ctx context.Context) (bool, error) {
	p.log.Info("starlark: has_capacity")

	m, err := p.CallExpectingDict(ctx, "has_capacity")
	if err != nil {
		return false, err
	}

	return starlib.MapField[bool](m, "has_capacity"), nil
}

func (p *starlarkProvisioner) Register(
	ctx context.Context,
	data provisioner.ManagementAccessData,
	credentialsChanged, restartOnFailure bool,
) (provisioner.Result, string, error) {
	p.log.Info("starlark: register")

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, "", fmt.Errorf("register: marshal data: %w", err)
	}

	result, m, err := p.CallAndParseResult(ctx, "register",
		starlib.GoToStarlark(dataMap),
		starlark.Bool(credentialsChanged),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, "", err
	}

	// Sentinel: script sets {"error": "needs-preprovisioning-image"} → typed error.
	if result.ErrorMessage == sentinelNeedsPreprovisioning {
		result.ErrorMessage = ""
		return result, starlib.MapField[string](m, "provID"), provisioner.ErrNeedsPreprovisioningImage
	}

	return result, starlib.MapField[string](m, "provID"), nil
}

func (p *starlarkProvisioner) PreprovisioningImageFormats(ctx context.Context) ([]metal3api.ImageFormat, error) {
	p.log.Info("starlark: preprovisioning_image_formats")

	val, err := p.CallScriptWithPublisher(ctx, "preprovisioning_image_formats", p.HostArgs())
	if err != nil {
		return nil, err
	}

	// None means "no preprovisioning image required" (per Provisioner interface).
	if val == starlark.None {
		return nil, nil
	}

	list, ok := val.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("preprovisioning_image_formats: expected list, got %s", val.Type())
	}

	formats := make([]metal3api.ImageFormat, list.Len())
	for i := range list.Len() {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, fmt.Errorf("preprovisioning_image_formats: element %d is not a string", i)
		}

		formats[i] = metal3api.ImageFormat(s)
	}

	return formats, nil
}

func (p *starlarkProvisioner) InspectHardware(
	ctx context.Context,
	data provisioner.InspectData,
	restartOnFailure, refresh, forceReboot bool,
) (provisioner.Result, bool, *metal3api.HardwareDetails, error) {
	p.log.Info("starlark: inspect_hardware")

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, false, nil, fmt.Errorf("inspect_hardware: marshal data: %w", err)
	}

	result, m, err := p.CallAndParseResult(ctx, "inspect_hardware",
		starlib.GoToStarlark(dataMap),
		starlark.Bool(restartOnFailure),
		starlark.Bool(refresh),
		starlark.Bool(forceReboot),
	)
	if err != nil {
		return result, false, nil, err
	}

	started := starlib.MapField[bool](m, "started")

	hwRaw := starlib.MapField[map[string]any](m, "hardwareDetails")
	if hwRaw == nil {
		return result, started, nil, nil
	}

	// Strict JSON passthrough keyed by metal3api.HardwareDetails tags.
	details, err := starlib.MapToStruct[metal3api.HardwareDetails](hwRaw)
	if err != nil {
		return result, started, nil, fmt.Errorf("inspect_hardware: parse hardwareDetails: %w", err)
	}

	return result, started, &details, nil
}

func (p *starlarkProvisioner) UpdateHardwareState(ctx context.Context) (provisioner.HardwareState, error) {
	p.log.Info("starlark: update_hardware_state")

	m, err := p.CallExpectingDict(ctx, "update_hardware_state")
	if err != nil {
		return provisioner.HardwareState{}, err
	}

	// Nil map from None return → zero state, no parsing needed.
	if m == nil {
		return provisioner.HardwareState{}, nil
	}

	// HardwareState has no JSON tags; script returns the field names verbatim.
	state, err := starlib.MapToStruct[provisioner.HardwareState](m)
	if err != nil {
		return provisioner.HardwareState{}, fmt.Errorf("update_hardware_state: parse: %w", err)
	}

	return state, nil
}

func (p *starlarkProvisioner) Adopt(ctx context.Context, data provisioner.AdoptData, restartOnFailure bool) (provisioner.Result, error) {
	p.log.Info("starlark: adopt")

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, fmt.Errorf("adopt: marshal data: %w", err)
	}

	result, _, err := p.CallAndParseResult(ctx, "adopt", starlib.GoToStarlark(dataMap), starlark.Bool(restartOnFailure))

	return result, err
}

// PascalCase key used by PrepareData/ServicingData for FirmwareConfig (no JSON tag on the Go field).
const FirmwareConfigMapKey = "FirmwareConfig"

// StripFirmwareConfig removes the deprecated FirmwareConfig key before scripts see it.
func StripFirmwareConfig(dataMap map[string]any) {
	delete(dataMap, FirmwareConfigMapKey)
}

func (p *starlarkProvisioner) Prepare(
	ctx context.Context,
	data provisioner.PrepareData,
	unprepared, restartOnFailure bool,
) (provisioner.Result, bool, error) {
	p.log.Info("starlark: prepare")

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, false, fmt.Errorf("prepare: marshal data: %w", err)
	}
	StripFirmwareConfig(dataMap)

	result, m, err := p.CallAndParseResult(ctx, "prepare",
		starlib.GoToStarlark(dataMap),
		starlark.Bool(unprepared),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, false, err
	}

	return result, starlib.MapField[bool](m, "started"), nil
}

func (p *starlarkProvisioner) Service(
	ctx context.Context,
	data provisioner.ServicingData,
	unprepared, restartOnFailure bool,
) (provisioner.Result, bool, error) {
	p.log.Info("starlark: service")

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, false, fmt.Errorf("service: marshal data: %w", err)
	}
	StripFirmwareConfig(dataMap)

	result, m, err := p.CallAndParseResult(ctx, "service",
		starlib.GoToStarlark(dataMap),
		starlark.Bool(unprepared),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, false, err
	}

	return result, starlib.MapField[bool](m, "started"), nil
}

func (p *starlarkProvisioner) Provision(
	ctx context.Context,
	data provisioner.ProvisionData,
	forceReboot bool,
) (provisioner.Result, error) {
	p.log.Info("starlark: provision")

	// Resolve cloud-init artifacts before calling Starlark so the script sees rendered strings.
	var (
		userData, networkData, metaData string
		err                             error
	)

	if data.HostConfig != nil {
		userData, err = data.HostConfig.UserData(ctx)
		if err != nil {
			return provisioner.Result{}, fmt.Errorf("resolving user data: %w", err)
		}

		networkData, err = data.HostConfig.NetworkData(ctx)
		if err != nil {
			return provisioner.Result{}, fmt.Errorf("resolving network data: %w", err)
		}

		metaData, err = data.HostConfig.MetaData(ctx)
		if err != nil {
			return provisioner.Result{}, fmt.Errorf("resolving meta data: %w", err)
		}
	}

	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, fmt.Errorf("provision: marshal data: %w", err)
	}
	// HostConfig is a method-only interface; replace with the rendered strings scripts can use.
	dataMap["HostConfig"] = map[string]any{
		"userData":    userData,
		"networkData": networkData,
		"metaData":    metaData,
	}

	result, _, err := p.CallAndParseResult(ctx, "provision", starlib.GoToStarlark(dataMap), starlark.Bool(forceReboot))

	return result, err
}

func (p *starlarkProvisioner) Deprovision(
	ctx context.Context,
	restartOnFailure bool,
	automatedCleaningMode metal3api.AutomatedCleaningMode,
) (provisioner.Result, error) {
	p.log.Info("starlark: deprovision")

	result, _, err := p.CallAndParseResult(ctx, "deprovision",
		starlark.Bool(restartOnFailure),
		starlark.String(string(automatedCleaningMode)),
	)
	if err != nil {
		return result, err
	}

	if needsReg, r := TranslateNeedsRegistration(result); needsReg {
		return r, provisioner.ErrNeedsRegistration
	}

	return result, nil
}

func (p *starlarkProvisioner) Delete(ctx context.Context) (provisioner.Result, error) {
	p.log.Info("starlark: delete")

	result, _, err := p.CallAndParseResult(ctx, "delete")
	if err != nil {
		return result, err
	}

	if needsReg, r := TranslateNeedsRegistration(result); needsReg {
		return r, provisioner.ErrNeedsRegistration
	}

	return result, nil
}

func (p *starlarkProvisioner) Detach(ctx context.Context, force bool) (provisioner.Result, error) {
	p.log.Info("starlark: detach")

	result, _, err := p.CallAndParseResult(ctx, "detach", starlark.Bool(force))

	return result, err
}

func (p *starlarkProvisioner) PowerOn(ctx context.Context, force bool) (provisioner.Result, error) {
	p.log.Info("starlark: power_on")

	result, _, err := p.CallAndParseResult(ctx, "power_on", starlark.Bool(force))

	return result, err
}

func (p *starlarkProvisioner) PowerOff(
	ctx context.Context,
	rebootMode metal3api.RebootMode,
	force bool,
	automatedCleaningMode metal3api.AutomatedCleaningMode,
) (provisioner.Result, error) {
	p.log.Info("starlark: power_off")

	result, _, err := p.CallAndParseResult(ctx, "power_off",
		starlark.String(string(rebootMode)),
		starlark.Bool(force),
		starlark.String(string(automatedCleaningMode)),
	)
	if err != nil {
		return result, err
	}

	if needsReg, r := TranslateNeedsRegistration(result); needsReg {
		return r, provisioner.ErrNeedsRegistration
	}

	return result, nil
}

// TranslateNeedsRegistration strips the needs-registration sentinel from a Result and reports whether it was present.
func TranslateNeedsRegistration(result provisioner.Result) (bool, provisioner.Result) {
	if result.ErrorMessage != sentinelNeedsRegistration {
		return false, result
	}

	result.ErrorMessage = ""

	return true, result
}

// Script-side return shape for get_firmware_settings: {"settings": {...}, "schema": {...}}.
type FirmwareSettingsResult struct {
	Settings metal3api.SettingsMap              `json:"settings,omitempty"`
	Schema   map[string]metal3api.SettingSchema `json:"schema,omitempty"`
}

func (p *starlarkProvisioner) GetFirmwareSettings(
	ctx context.Context,
	includeSchema bool,
) (metal3api.SettingsMap, map[string]metal3api.SettingSchema, error) {
	p.log.Info("starlark: get_firmware_settings")

	val, err := p.CallScriptWithPublisher(ctx, "get_firmware_settings",
		append(p.HostArgs(), starlark.Bool(includeSchema)),
	)
	if err != nil {
		return nil, nil, err
	}

	if val == starlark.None {
		return nil, nil, nil
	}

	d, ok := val.(*starlark.Dict)
	if !ok {
		return nil, nil, fmt.Errorf("get_firmware_settings: expected dict, got %s", val.Type())
	}

	m, ok := starlib.ToGo(d).(map[string]any)
	if !ok {
		return nil, nil, errors.New("get_firmware_settings: result is not a map")
	}

	// Strict JSON passthrough; keys match metal3api.SettingSchema tags.
	parsed, err := starlib.MapToStruct[FirmwareSettingsResult](m)
	if err != nil {
		return nil, nil, fmt.Errorf("get_firmware_settings: parse: %w", err)
	}

	// Always return a non-nil Settings so callers can tell success-empty from unrequested.
	settings := parsed.Settings
	if settings == nil {
		settings = metal3api.SettingsMap{}
	}

	if !includeSchema {
		return settings, nil, nil
	}

	return settings, parsed.Schema, nil
}

func (p *starlarkProvisioner) AddBMCEventSubscriptionForNode(
	ctx context.Context,
	subscription *metal3api.BMCEventSubscription,
	httpHeaders provisioner.HTTPHeaders,
) (provisioner.Result, error) {
	p.log.Info("starlark: add_bmc_event_subscription")

	subMap, err := starlib.StructToMap(subscription)
	if err != nil {
		return provisioner.Result{}, fmt.Errorf("add_bmc_event_subscription: marshal subscription: %w", err)
	}
	// Secret-backed HTTPHeaders aren't part of the CRD; merge at the top level.
	subMap["httpHeaders"] = httpHeaders

	result, _, err := p.CallAndParseResult(ctx, "add_bmc_event_subscription", starlib.GoToStarlark(subMap))

	return result, err
}

func (p *starlarkProvisioner) RemoveBMCEventSubscriptionForNode(
	ctx context.Context,
	subscription metal3api.BMCEventSubscription,
) (provisioner.Result, error) {
	p.log.Info("starlark: remove_bmc_event_subscription")

	subMap, err := starlib.StructToMap(subscription)
	if err != nil {
		return provisioner.Result{}, fmt.Errorf("remove_bmc_event_subscription: marshal subscription: %w", err)
	}

	result, _, err := p.CallAndParseResult(ctx, "remove_bmc_event_subscription", starlib.GoToStarlark(subMap))

	return result, err
}

// IsFirmwareUnsupported reports whether a Starlark return dict carries the firmware-updates-unsupported sentinel.
func IsFirmwareUnsupported(val starlark.Value) bool {
	d, ok := val.(*starlark.Dict)
	if !ok {
		return false
	}

	m, ok := starlib.ToGo(d).(map[string]any)
	if !ok {
		return false
	}

	if starlib.MapField[string](m, "error") != sentinelFirmwareUnsupported {
		return false
	}

	for _, k := range []string{"dirty", "requeue_after_seconds"} {
		if _, set := m[k]; set {
			log.V(1).Info("starlark get_firmware_components: sentinel + extra key ignored",
				"sentinel", sentinelFirmwareUnsupported,
				"ignored_key", k,
			)
		}
	}

	return true
}

func (p *starlarkProvisioner) GetFirmwareComponents(ctx context.Context) ([]metal3api.FirmwareComponentStatus, error) {
	p.log.Info("starlark: get_firmware_components")

	val, err := p.CallScriptWithPublisher(ctx, "get_firmware_components", p.HostArgs())
	if err != nil {
		return nil, err
	}

	// None = "no components this cycle"; distinct from the unsupported sentinel below.
	if val == starlark.None {
		return nil, nil
	}

	// Sentinel {"error": "firmware-updates-unsupported"} → typed error, no HostFirmwareComponents.
	if IsFirmwareUnsupported(val) {
		return nil, provisioner.ErrFirmwareUpdateUnsupported
	}

	list, ok := val.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("get_firmware_components: expected list, got %s", val.Type())
	}

	// Strict JSON passthrough; keys match metal3api.FirmwareComponentStatus tags.
	items, ok := starlib.ToGo(list).([]any)
	if !ok {
		return nil, errors.New("get_firmware_components: result is not a list")
	}

	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("get_firmware_components: marshal: %w", err)
	}

	var out []metal3api.FirmwareComponentStatus
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("get_firmware_components: parse: %w", err)
	}

	return out, nil
}

func (p *starlarkProvisioner) GetDataImageStatus(ctx context.Context) (bool, error) {
	p.log.Info("starlark: get_data_image_status")

	m, err := p.CallExpectingDict(ctx, "get_data_image_status")
	if err != nil {
		return false, err
	}

	// Sentinel: node reserved by another Ironic task → typed retry-without-error.
	if starlib.MapField[string](m, "error") == sentinelNodeBusy {
		return false, provisioner.ErrNodeIsBusy
	}

	return starlib.MapField[bool](m, "attached"), nil
}

func (p *starlarkProvisioner) AttachDataImage(ctx context.Context, url string) error {
	p.log.Info("starlark: attach_data_image")

	return p.CallVoid(ctx, "attach_data_image", starlark.String(url))
}

func (p *starlarkProvisioner) DetachDataImage(ctx context.Context) error {
	p.log.Info("starlark: detach_data_image")

	return p.CallVoid(ctx, "detach_data_image")
}

// PublishScriptError emits a StarlarkScriptError event so failures in error-less methods reach operators.
func (p *starlarkProvisioner) PublishScriptError(method string, err error) {
	if p.publisher == nil {
		return
	}

	p.publisher("StarlarkScriptError", fmt.Sprintf("%s: %s", method, err.Error()))
}

func (p *starlarkProvisioner) HasPowerFailure(ctx context.Context) bool {
	p.log.Info("starlark: has_power_failure")

	val, err := p.CallScriptWithPublisher(ctx, "has_power_failure", p.HostArgs())
	if err != nil {
		p.log.Error(err, "has_power_failure failed")
		p.PublishScriptError("has_power_failure", err)

		return false
	}

	b, ok := val.(starlark.Bool)
	if !ok {
		return false
	}

	return bool(b)
}

func (p *starlarkProvisioner) GetHealth(ctx context.Context) string {
	p.log.Info("starlark: get_health")

	val, err := p.CallScriptWithPublisher(ctx, "get_health", p.HostArgs())
	if err != nil {
		p.log.Error(err, "get_health failed")
		p.PublishScriptError("get_health", err)

		return ""
	}

	s, ok := starlark.AsString(val)
	if !ok {
		return ""
	}

	return s
}
