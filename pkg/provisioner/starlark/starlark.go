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

// Package starlark delegates each Provisioner method to a named function in a user-supplied Starlark script.
package starlark

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/starlark/starlib"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/starlark/starscript"
	"go.starlark.net/starlark"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Sentinel "error" dict values that scripts set to trigger typed controller behavior.
const (
	sentinelFirmwareUnsupported  = "firmware-updates-unsupported"
	sentinelNeedsPreprovisioning = "needs-preprovisioning-image"
	sentinelNeedsRegistration    = "needs-registration"
	sentinelNodeBusy             = "node-busy"
)

var log = logf.Log.WithName("provisioner").WithName("starlark")

// Frozen script globals shared across per-host provisioner instances.
type starlarkProvisionerFactory struct {
	globals    starlark.StringDict
	scriptPath string
	log        logr.Logger
}

// Per-host state shared by every provisioner.Provisioner method (defined in provisioner.go).
type starlarkProvisioner struct {
	globals   starlark.StringDict
	publisher provisioner.EventPublisher
	hostData  provisioner.HostData
	log       logr.Logger
}

// NewProvisionerFactory loads the Starlark script and validates that every required function is callable.
func NewProvisionerFactory(scriptPath string) (provisioner.Factory, error) {
	globals, err := starscript.LoadScript(scriptPath, starlib.Builtins())
	if err != nil {
		return nil, fmt.Errorf("starlark provisioner: %w", err)
	}

	if err := starscript.ValidateRequiredFunctions(globals); err != nil {
		return nil, fmt.Errorf("starlark provisioner: script %s %w", scriptPath, err)
	}

	return &starlarkProvisionerFactory{
		globals:    globals,
		scriptPath: scriptPath,
		log:        log,
	}, nil
}

// NewProvisioner creates a per-host starlark provisioner (ctx unused; present for the Factory interface).
func (f *starlarkProvisionerFactory) NewProvisioner(
	_ context.Context,
	hostData provisioner.HostData,
	publisher provisioner.EventPublisher,
) (provisioner.Provisioner, error) {
	return &starlarkProvisioner{
		globals:   f.globals,
		hostData:  hostData,
		log:       f.log.WithValues("host", hostData.ObjectMeta.Name),
		publisher: publisher,
	}, nil
}

// CallScriptWithPublisher runs a script function with ctx/publisher/logger in thread-locals and redacts the BMC password from any error.
func (p *starlarkProvisioner) CallScriptWithPublisher(ctx context.Context, name string, args starlark.Tuple) (starlark.Value, error) {
	thread := &starlark.Thread{Name: name}
	if p.publisher != nil {
		thread.SetLocal(starlib.PublisherThreadLocal, p.publisher)
	}
	thread.SetLocal(starlib.LoggerThreadLocal, p.log)

	// Per-call ctx so the watcher goroutine exits whenever the call returns.
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	thread.SetLocal(starlib.CtxThreadLocal, callCtx)
	go func() {
		<-callCtx.Done()
		thread.Cancel(callCtx.Err().Error())
	}()

	val, err := starscript.CallOnThread(thread, p.globals, name, args)
	if err != nil {
		// Redact the BMC password from error messages before it reaches logs/status/events.
		if pw := p.hostData.BMCCredentials.Password; pw != "" {
			err = errors.New(strings.ReplaceAll(err.Error(), pw, "***"))
		}
	}

	return val, err
}

// HostArgs builds the standard connection arguments passed to every Starlark function.
func (p *starlarkProvisioner) HostArgs() starlark.Tuple {
	m, err := starlib.StructToMap(p.hostData)
	if err != nil {
		// HostData is always serializable; failure here is a programming error.
		panic(fmt.Sprintf("HostArgs: %v", err))
	}

	return starlark.Tuple{starlib.GoToStarlark(m)}
}

// CallAndParseResult invokes a script function and parses its dict into provisioner.Result (raw map returned for extras).
func (p *starlarkProvisioner) CallAndParseResult(
	ctx context.Context,
	name string,
	extraArgs ...starlark.Value,
) (provisioner.Result, map[string]any, error) {
	args := p.HostArgs()
	args = append(args, extraArgs...)

	val, err := p.CallScriptWithPublisher(ctx, name, args)
	if err != nil {
		return provisioner.Result{}, nil, err
	}

	d, ok := val.(*starlark.Dict)
	if !ok {
		return provisioner.Result{}, nil, fmt.Errorf("%s: expected dict return, got %s", name, val.Type())
	}

	m, ok := starlib.ToGo(d).(map[string]any)
	if !ok {
		return provisioner.Result{}, nil, fmt.Errorf("%s: result is not a map", name)
	}

	result := provisioner.Result{
		Dirty:        starlib.MapField[bool](m, "dirty"),
		RequeueAfter: starlib.MapFieldDuration(m, "requeue_after_seconds"),
		ErrorMessage: starlib.MapField[string](m, "error"),
	}

	return result, m, nil
}

// CallVoid calls a Starlark function that returns no meaningful value (or None).
func (p *starlarkProvisioner) CallVoid(ctx context.Context, name string, extraArgs ...starlark.Value) error {
	args := p.HostArgs()
	args = append(args, extraArgs...)

	_, err := p.CallScriptWithPublisher(ctx, name, args)

	return err
}

// CallExpectingDict invokes a query-style script function; returns (nil, nil) when the script returns None.
func (p *starlarkProvisioner) CallExpectingDict(ctx context.Context, name string) (map[string]any, error) {
	val, err := p.CallScriptWithPublisher(ctx, name, p.HostArgs())
	if err != nil {
		return nil, err
	}

	// None is the "no info this cycle" signal; callers read zero values via MapField[T].
	if val == starlark.None {
		return nil, nil //nolint:nilnil // intentional "no info" signal
	}

	d, ok := val.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("%s: expected dict, got %s", name, val.Type())
	}

	m, ok := starlib.ToGo(d).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: result is not a map", name)
	}

	return m, nil
}
