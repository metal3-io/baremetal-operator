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

// Provisioner interface methods. Each delegates to a named Starlark function.

package starlark

import (
	"context"
	"errors"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"go.starlark.net/starlark"
)

func (p *starlarkProvisioner) HasCapacity(ctx context.Context) (bool, error) {
	p.log.Info("starlark: has_capacity")

	m, err := p.CallExpectingDict(ctx, "has_capacity")
	if err != nil {
		return false, err
	}

	// A missing has_capacity would silently read false and stall the host, so require it.
	hasCapacity, err := starlib.MapFieldStrict[bool](m, "has_capacity")
	if err != nil {
		return false, fmt.Errorf("has_capacity: %w", err)
	}

	return hasCapacity, nil
}

func (p *starlarkProvisioner) Register(
	ctx context.Context,
	data provisioner.ManagementAccessData,
	credentialsChanged, restartOnFailure bool,
) (provisioner.Result, string, error) {
	p.log.Info("starlark: register")

	result, m, err := p.CallWithData(ctx, "register", data,
		starlark.Bool(credentialsChanged),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, "", err
	}

	// When the script sets error to the needs preprovisioning sentinel, return the typed error.
	if result.ErrorMessage == SentinelNeedsPreprovisioning {
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

	result, m, err := p.CallWithData(ctx, "inspect_hardware", data,
		starlark.Bool(restartOnFailure),
		starlark.Bool(refresh),
		starlark.Bool(forceReboot),
	)
	if err != nil {
		return result, false, nil, err
	}

	started, err := starlib.MapFieldTyped[bool](m, "started")
	if err != nil {
		return result, false, nil, fmt.Errorf("inspect_hardware: %w", err)
	}

	hwRaw, err := starlib.MapFieldTyped[map[string]any](m, "hardwareDetails")
	if err != nil {
		return result, started, nil, fmt.Errorf("inspect_hardware: %w", err)
	}
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

	// Nil map from None return yields zero state, no parsing needed.
	if m == nil {
		return provisioner.HardwareState{}, nil
	}

	// An error field means the script could not read state, so surface it instead of stale power info.
	if msg := starlib.MapField[string](m, "error"); msg != "" {
		return provisioner.HardwareState{}, errors.New(MaskSecret(msg, p.hostData.BMCCredentials.Password))
	}

	// HardwareState has no JSON tags. The script returns the field names verbatim.
	state, err := starlib.MapToStruct[provisioner.HardwareState](m)
	if err != nil {
		return provisioner.HardwareState{}, fmt.Errorf("update_hardware_state: parse: %w", err)
	}

	return state, nil
}

func (p *starlarkProvisioner) Adopt(ctx context.Context, data provisioner.AdoptData, restartOnFailure bool) (provisioner.Result, error) {
	p.log.Info("starlark: adopt")

	result, _, err := p.CallWithData(ctx, "adopt", data, starlark.Bool(restartOnFailure))

	return result, err
}

func (p *starlarkProvisioner) Prepare(
	ctx context.Context,
	data provisioner.PrepareData,
	unprepared, restartOnFailure bool,
) (provisioner.Result, bool, error) {
	p.log.Info("starlark: prepare")

	result, m, err := p.CallWithData(ctx, "prepare", data,
		starlark.Bool(unprepared),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, false, err
	}

	started, err := starlib.MapFieldTyped[bool](m, "started")
	if err != nil {
		return result, false, fmt.Errorf("prepare: %w", err)
	}

	return result, started, nil
}

func (p *starlarkProvisioner) Service(
	ctx context.Context,
	data provisioner.ServicingData,
	unprepared, restartOnFailure bool,
) (provisioner.Result, bool, error) {
	p.log.Info("starlark: service")

	result, m, err := p.CallWithData(ctx, "service", data,
		starlark.Bool(unprepared),
		starlark.Bool(restartOnFailure),
	)
	if err != nil {
		return result, false, err
	}

	started, err := starlib.MapFieldTyped[bool](m, "started")
	if err != nil {
		return result, false, fmt.Errorf("service: %w", err)
	}

	return result, started, nil
}

func (p *starlarkProvisioner) Provision(
	ctx context.Context,
	data provisioner.ProvisionData,
	forceReboot bool,
) (provisioner.Result, error) {
	p.log.Info("starlark: provision")

	// Resolve cloud init artifacts before calling Starlark so the script sees rendered strings.
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
	// HostConfig is a method only interface. Replace with the rendered strings scripts can use.
	dataMap["HostConfig"] = map[string]any{
		"userData":    userData,
		"networkData": networkData,
		"metaData":    metaData,
	}

	result, _, err := p.CallAndParseResult(ctx, "provision", starlib.GoToStarlark(dataMap), starlark.Bool(forceReboot))

	return result, err
}

// ErrorFromResult turns a script reported error into a Go error. The controller
// discards the Result for some methods and reads only the error.
func ErrorFromResult(method string, result provisioner.Result) error {
	if result.ErrorMessage == "" {
		return nil
	}

	return fmt.Errorf("%s: %s", method, result.ErrorMessage)
}

// TranslateNeedsRegistration strips the needs registration sentinel from a Result and reports whether it was present.
func TranslateNeedsRegistration(result provisioner.Result) (bool, provisioner.Result) {
	if result.ErrorMessage != SentinelNeedsRegistration {
		return false, result
	}

	result.ErrorMessage = ""

	return true, result
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

	// Only once deprovisioning is done. Clearing earlier would delete a route the
	// script just registered, and clearing on every pass would race its re register.
	if !result.Dirty {
		p.clearServeRoutes(ctx)
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

	if !result.Dirty {
		p.clearServeRoutes(ctx)
	}

	// A needs registration node is already gone, so deletion is complete (mirrors ironic Delete).
	if needsReg, _ := TranslateNeedsRegistration(result); needsReg {
		return provisioner.Result{}, nil
	}

	// actionDeleting reads only the error, so a Result carried failure would drop
	// the finalizer and leave the backend node behind.
	return result, ErrorFromResult("delete", result)
}

func (p *starlarkProvisioner) Detach(ctx context.Context, force bool) (provisioner.Result, error) {
	p.log.Info("starlark: detach")

	result, _, err := p.CallAndParseResult(ctx, "detach", starlark.Bool(force))
	if err != nil {
		return result, err
	}

	// A needs registration node is already gone, so detach is complete (mirrors ironic Detach).
	if needsReg, _ := TranslateNeedsRegistration(result); needsReg {
		return provisioner.Result{}, nil
	}

	return result, ErrorFromResult("detach", result)
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

// Script side return shape for get_firmware_settings holds settings and schema dicts.
type FirmwareSettingsResult struct {
	Settings metal3api.SettingsMap              `json:"settings,omitzero"`
	Schema   map[string]metal3api.SettingSchema `json:"schema,omitzero"`
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

	m, err := starlib.AsMap("get_firmware_settings", val)
	if err != nil {
		return nil, nil, err
	}

	// Strict JSON passthrough. Keys match metal3api.SettingSchema tags.
	parsed, err := starlib.MapToStruct[FirmwareSettingsResult](m)
	if err != nil {
		return nil, nil, fmt.Errorf("get_firmware_settings: parse: %w", err)
	}

	// Always return a nonnil Settings so callers can tell success empty from unrequested.
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
	// Secret backed HTTPHeaders aren't part of the CRD. Merge at the top level.
	subMap["httpHeaders"] = httpHeaders

	result, m, err := p.CallAndParseResult(ctx, "add_bmc_event_subscription", starlib.GoToStarlark(subMap))
	if err != nil {
		return result, err
	}

	// Propagate the created subscription id so RemoveBMCEventSubscriptionForNode can delete it.
	if id := starlib.MapField[string](m, "subscriptionID"); id != "" {
		subscription.Status.SubscriptionID = id
	}

	// The subscription controller discards the Result, so a failure has to be an error.
	return result, ErrorFromResult("add_bmc_event_subscription", result)
}

func (p *starlarkProvisioner) RemoveBMCEventSubscriptionForNode(
	ctx context.Context,
	subscription metal3api.BMCEventSubscription,
) (provisioner.Result, error) {
	p.log.Info("starlark: remove_bmc_event_subscription")

	result, _, err := p.CallWithData(ctx, "remove_bmc_event_subscription", subscription)
	if err != nil {
		return result, err
	}

	return result, ErrorFromResult("remove_bmc_event_subscription", result)
}

// IsFirmwareUnsupported reports whether a Starlark return dict carries the firmware updates unsupported sentinel.
func IsFirmwareUnsupported(val starlark.Value) bool {
	m, err := starlib.AsMap("get_firmware_components", val)
	if err != nil {
		return false
	}

	if starlib.MapField[string](m, "error") != SentinelFirmwareUnsupported {
		return false
	}

	for _, k := range []string{"dirty", "requeue_after_seconds"} {
		if _, set := m[k]; set {
			log.V(1).Info("starlark get_firmware_components: sentinel + extra key ignored",
				"sentinel", SentinelFirmwareUnsupported,
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

	// None = "no components this cycle".
	if val == starlark.None {
		return nil, nil
	}

	// The firmware unsupported sentinel yields an empty component list. The BMH controller
	// creates HostFirmwareComponents only when components exist, so no components signals unsupported.
	if IsFirmwareUnsupported(val) {
		return nil, nil
	}

	list, ok := val.(*starlark.List)
	if !ok {
		// A dict carrying a plain error is how a script reports a read failure, so
		// surface that message instead of the generic shape mismatch.
		if m, aerr := starlib.AsMap("get_firmware_components", val); aerr == nil {
			if msg := starlib.MapField[string](m, "error"); msg != "" {
				return nil, errors.New(MaskSecret(msg, p.hostData.BMCCredentials.Password))
			}
		}

		return nil, fmt.Errorf("get_firmware_components: expected list, got %s", val.Type())
	}

	// Strict JSON passthrough. Keys match metal3api.FirmwareComponentStatus tags.
	items, ok := starlib.ToGo(list).([]any)
	if !ok {
		return nil, errors.New("get_firmware_components: result is not a list")
	}

	out, err := starlib.SliceToStruct[metal3api.FirmwareComponentStatus](items)
	if err != nil {
		return nil, fmt.Errorf("get_firmware_components: %w", err)
	}

	return out, nil
}

func (p *starlarkProvisioner) GetDataImageStatus(ctx context.Context) (bool, error) {
	p.log.Info("starlark: get_data_image_status")

	m, err := p.CallExpectingDict(ctx, "get_data_image_status")
	if err != nil {
		return false, err
	}

	// When a node is reserved by another Ironic task, return a typed retry without error.
	if starlib.MapField[string](m, "error") == SentinelNodeBusy {
		return false, provisioner.ErrNodeIsBusy
	}

	// A missing attached would silently read false and leak the media, so require it.
	attached, err := starlib.MapFieldStrict[bool](m, "attached")
	if err != nil {
		return false, fmt.Errorf("get_data_image_status: %w", err)
	}

	return attached, nil
}

func (p *starlarkProvisioner) AttachDataImage(ctx context.Context, url string) error {
	p.log.Info("starlark: attach_data_image")

	// The controller records the image as attached on a nil error, so a script
	// reported failure has to surface or the host oscillates on every reconcile.
	return p.CallVoid(ctx, "attach_data_image", starlark.String(url))
}

func (p *starlarkProvisioner) DetachDataImage(ctx context.Context) error {
	p.log.Info("starlark: detach_data_image")

	return p.CallVoid(ctx, "detach_data_image")
}

// PublishScriptError emits a StarlarkScriptError event so failures in errorless methods reach operators.
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
