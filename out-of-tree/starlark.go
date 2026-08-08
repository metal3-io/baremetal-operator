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

// Package starlark delegates each Provisioner method to a named function in a user supplied Starlark script.
package starlark

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
	"go.starlark.net/starlark"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Sentinel "error" dict values that scripts set to trigger typed controller behavior.
const (
	SentinelFirmwareUnsupported  = "firmware-updates-unsupported"
	SentinelNeedsPreprovisioning = "needs-preprovisioning-image"
	SentinelNeedsRegistration    = "needs-registration"
	SentinelNodeBusy             = "node-busy"
)

// IsSentinel reports whether s is one of the control sentinels that drive typed behavior.
func IsSentinel(s string) bool {
	switch s {
	case SentinelFirmwareUnsupported, SentinelNeedsPreprovisioning, SentinelNeedsRegistration, SentinelNodeBusy:
		return true
	default:
		return false
	}
}

var log = logf.Log.WithName("provisioner").WithName("starlark")

// Frozen script globals shared across per host provisioner instances.
type starlarkProvisionerFactory struct {
	globals        starlark.StringDict
	scriptPath     string
	log            logr.Logger
	secretResolver starlib.HostResolver
	callback       CallbackConfig
	serveEnabled   bool
}

// Per host state shared by every provisioner.Provisioner method (defined in provisioner.go).
type starlarkProvisioner struct {
	globals        starlark.StringDict
	publisher      provisioner.EventPublisher
	hostData       provisioner.HostData
	log            logr.Logger
	secretResolver starlib.HostResolver
	callbackToken  string
	// baseURL is the single listener address behind both the callback and serve URLs.
	baseURL      string
	serveEnabled bool
}

// NewProvisionerFactory loads the Starlark script and validates that every required function is callable.
func NewProvisionerFactory(scriptPath string, secretResolver starlib.HostResolver) (provisioner.Factory, error) {
	globals, err := starscript.LoadScript(scriptPath, starlib.Builtins())
	if err != nil {
		return nil, fmt.Errorf("starlark provisioner: %w", err)
	}

	if err := starscript.ValidateRequiredFunctions(globals); err != nil {
		return nil, fmt.Errorf("starlark provisioner: script %s %w", scriptPath, err)
	}

	cfg := LoadCallbackConfig()

	factory := &starlarkProvisionerFactory{
		globals:        globals,
		scriptPath:     scriptPath,
		log:            log,
		secretResolver: secretResolver,
	}

	// Enable the facilities only once the listener is actually serving, so scripts
	// never advertise a dead endpoint.
	if cfg.Enabled() {
		if kr, ok := secretResolver.(*KubeHostResolver); !ok {
			log.Info("plugin listener disabled, resolver has no support")
		} else {
			server := &PluginServer{Config: cfg, Resolver: kr, Serve: kr, Log: log.WithName("http")}

			if err := server.Start(); err != nil {
				log.Error(err, "plugin listener failed to start, facilities disabled")
			} else {
				factory.callback = cfg
				factory.serveEnabled = true
			}
		}
	}

	return factory, nil
}

// NewProvisioner creates a per host starlark provisioner (ctx unused and present for the Factory interface).
func (f *starlarkProvisionerFactory) NewProvisioner(
	_ context.Context,
	hostData provisioner.HostData,
	publisher provisioner.EventPublisher,
) (provisioner.Provisioner, error) {
	// Mask the BMC password in every output channel this provisioner drives.
	pw := hostData.BMCCredentials.Password

	// Precompute the per host callback token so builtins never touch the crypto.
	// An empty password cannot key a safe token, so callbacks stay off for that host.
	token := ""
	if f.callback.Enabled() && pw != "" {
		token = SynthesizeToken(hostData.ObjectMeta.Namespace, hostData.ObjectMeta.Name, hostData.BMCCredentials.Username, pw)
	}

	return &starlarkProvisioner{
		globals:        f.globals,
		hostData:       hostData,
		log:            MaskingLogger(f.log.WithValues("host", hostData.ObjectMeta.Name), pw),
		publisher:      MaskingPublisher(publisher, pw),
		secretResolver: f.secretResolver,
		callbackToken:  token,
		baseURL:        f.callback.BaseURL,
		serveEnabled:   f.serveEnabled,
	}, nil
}

// RedactedError hides a substring (the BMC password) from an error's message
// while preserving the wrapped chain for errors.Is and errors.As.
type RedactedError struct {
	err      error
	password string
}

func (e *RedactedError) Error() string {
	return MaskSecret(e.err.Error(), e.password)
}

func (e *RedactedError) Unwrap() error { return e.err }

// CallScriptWithPublisher runs a script function with ctx/publisher/logger in thread locals and redacts the BMC password from any error.
func (p *starlarkProvisioner) CallScriptWithPublisher(ctx context.Context, name string, args starlark.Tuple) (starlark.Value, error) {
	thread := &starlark.Thread{Name: name}
	if p.publisher != nil {
		thread.SetLocal(starlib.PublisherThreadLocal, p.publisher)
	}
	thread.SetLocal(starlib.LoggerThreadLocal, p.log)
	if p.secretResolver != nil {
		thread.SetLocal(starlib.HostResolverThreadLocal, p.secretResolver)
	}
	thread.SetLocal(starlib.HostNamespaceThreadLocal, p.hostData.ObjectMeta.Namespace)
	thread.SetLocal(starlib.HostNameThreadLocal, p.hostData.ObjectMeta.Name)
	if p.callbackToken != "" {
		thread.SetLocal(starlib.CallbackThreadLocal, &starlib.CallbackContext{BaseURL: p.baseURL, Token: p.callbackToken})
	}
	if p.serveEnabled {
		thread.SetLocal(starlib.ServeThreadLocal, &starlib.ServeContext{
			BaseURL: p.baseURL,
			UID:     string(p.hostData.ObjectMeta.UID),
		})
	}

	// Per call ctx so the watcher goroutine exits whenever the call returns.
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	thread.SetLocal(starlib.CtxThreadLocal, callCtx)
	go func() {
		<-callCtx.Done()
		thread.Cancel(callCtx.Err().Error())
	}()

	val, err := starscript.CallOnThread(thread, p.globals, name, args)
	if err != nil {
		// Redact the BMC password while preserving the wrapped error chain so
		// errors.Is and errors.As keep working downstream.
		if pw := p.hostData.BMCCredentials.Password; pw != "" {
			err = &RedactedError{err: err, password: pw}
		}
	}

	return val, err
}

// HostArgs builds the standard connection arguments passed to every Starlark function.
func (p *starlarkProvisioner) HostArgs() starlark.Tuple {
	m, err := starlib.StructToMap(p.hostData)
	if err != nil {
		// HostData is always serializable. Failure here is a programming error.
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

	m, err := starlib.AsMap(name, val)
	if err != nil {
		return provisioner.Result{}, nil, err
	}

	errMsg := starlib.MapField[string](m, "error")
	// Mask a real error message, but leave exact sentinels intact so typed behavior still triggers.
	if !IsSentinel(errMsg) {
		errMsg = MaskSecret(errMsg, p.hostData.BMCCredentials.Password)
	}

	result := provisioner.Result{
		Dirty:        starlib.MapField[bool](m, "dirty"),
		RequeueAfter: starlib.MapFieldDuration(m, "requeue_after_seconds"),
		ErrorMessage: errMsg,
	}

	// The controller only requeues when Dirty is set, so a requeue delay implies Dirty.
	if result.RequeueAfter > 0 {
		result.Dirty = true
	}

	return result, m, nil
}

// CallWithData marshals a provisioner data struct and calls name with it after
// the host. Callers that amend the map first use CallAndParseResult directly.
func (p *starlarkProvisioner) CallWithData(
	ctx context.Context,
	name string,
	data any,
	extraArgs ...starlark.Value,
) (provisioner.Result, map[string]any, error) {
	dataMap, err := starlib.StructToMap(data)
	if err != nil {
		return provisioner.Result{}, nil, fmt.Errorf("%s: marshal data: %w", name, err)
	}

	return p.CallAndParseResult(ctx, name, append([]starlark.Value{starlib.GoToStarlark(dataMap)}, extraArgs...)...)
}

// CallVoid calls a Starlark function that returns no meaningful value (or None).
func (p *starlarkProvisioner) CallVoid(ctx context.Context, name string, extraArgs ...starlark.Value) error {
	args := p.HostArgs()
	args = append(args, extraArgs...)

	_, err := p.CallScriptWithPublisher(ctx, name, args)

	return err
}

// CallExpectingDict invokes a query style script function and returns (nil, nil) when the script returns None.
func (p *starlarkProvisioner) CallExpectingDict(ctx context.Context, name string) (map[string]any, error) {
	val, err := p.CallScriptWithPublisher(ctx, name, p.HostArgs())
	if err != nil {
		return nil, err
	}

	// None is the "no info this cycle" signal and callers read zero values via MapField[T].
	if val == starlark.None {
		return nil, nil //nolint:nilnil // intentional "no info" signal
	}

	return starlib.AsMap(name, val)
}
