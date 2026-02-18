# -*- mode: Python -*-

update_settings(k8s_upsert_timeout_secs=60)  # on first tilt up, often can take longer than 30 seconds

load("ext://uibutton", "cmd_button", "location", "text_input")
# set defaults
settings = {
    "allowed_contexts": [
        "kind-bmo"
    ],
    "preload_images_for_kind": True,
    "kind_cluster_name": "bmo",
    "enable_providers": [],
}

keys = []

always_enable_providers = ["metal3-bmo"]
providers = {}
extra_args = settings.get("extra_args", {})

# global settings
settings.update(read_json(
    "tilt-settings.json",
    default = {},
))

if settings.get("trigger_mode") == "manual":
    trigger_mode(TRIGGER_MODE_MANUAL)

if "allowed_contexts" in settings:
    allow_k8s_contexts(settings.get("allowed_contexts"))

if "default_registry" in settings:
    default_registry(settings.get("default_registry"))

def load_provider_tiltfiles(provider_repos):
    for repo in provider_repos:
        file = repo + "/tilt-provider.json"
        provider_details = read_json(file, default = {})
        if type(provider_details) != type([]):
            provider_details = [provider_details]
        for item in provider_details:
            provider_name = item["name"]
            provider_config = item["config"]
            if "context" in provider_config:
                provider_config["context"] = repo + "/" + provider_config["context"]
            else:
                provider_config["context"] = repo
            if "kustomize_config" not in provider_config:
                provider_config["kustomize_config"] = True
            if "go_main" not in provider_config:
                provider_config["go_main"] = "main.go"
            providers[provider_name] = provider_config

def validate_auth():
    substitutions = settings.get("kustomize_substitutions", {})
    missing = [k for k in keys if k not in substitutions]
    if missing:
        fail("missing kustomize_substitutions keys {} in tilt-setting.json".format(missing))

tilt_helper_dockerfile_header = """
# Tilt image
FROM golang:1.25 as tilt-helper
# Support live reloading with Tilt
RUN wget --output-document /restart.sh --quiet https://raw.githubusercontent.com/windmilleng/rerun-process-wrapper/master/restart.sh  && \
    wget --output-document /start.sh --quiet https://raw.githubusercontent.com/windmilleng/rerun-process-wrapper/master/start.sh && \
    chmod +x /start.sh && chmod +x /restart.sh
"""

tilt_dockerfile_header = """
FROM gcr.io/distroless/base:debug as tilt
WORKDIR /
COPY --from=tilt-helper /start.sh .
COPY --from=tilt-helper /restart.sh .
COPY manager .
"""

# Configures a provider by doing the following:
#
# 1. Enables a local_resource go build of the provider's manager binary
# 2. Configures a docker build for the provider, with live updating of the manager binary
# 3. Runs kustomize for the provider's config/ and applies it
def enable_provider(name):
    p = providers.get(name)

    name = p.get("name", name)
    context = p.get("context")
    go_main = p.get("go_main")

    # Prefix each live reload dependency with context. For example, for if the context is
    # test/infra/docker and main.go is listed as a dep, the result is test/infra/docker/main.go. This adjustment is
    # needed so Tilt can watch the correct paths for changes.
    live_reload_deps = []
    for d in p.get("live_reload_deps", []):
        live_reload_deps.append(context + "/" + d)

    # Set up a local_resource build of the provider's manager binary. The provider is expected to have a main.go in
    # manager_build_path or the main.go must be provided via go_main option. The binary is written to .tiltbuild/manager.
    local_resource(
        name + "_manager",
        cmd = "cd " + context + ';mkdir -p .tiltbuild;CGO_ENABLED=0 go build -ldflags \'-extldflags "-static"\' -o .tiltbuild/manager ' + go_main,
        deps = live_reload_deps,
    )

    additional_docker_helper_commands = p.get("additional_docker_helper_commands", "")
    additional_docker_build_commands = p.get("additional_docker_build_commands", "")

    dockerfile_contents = "\n".join([
        tilt_helper_dockerfile_header,
        additional_docker_helper_commands,
        tilt_dockerfile_header,
        additional_docker_build_commands,
    ])

    # Set up an image build for the provider. The live update configuration syncs the output from the local_resource
    # build into the container.
    entrypoint = ["sh", "/start.sh", "/manager"]
    provider_args = extra_args.get(name)
    if provider_args:
        entrypoint.extend(provider_args)

    docker_build(
        ref = p.get("image"),
        context = context + "/.tiltbuild/",
        dockerfile_contents = dockerfile_contents,
        target = "tilt",
        entrypoint = entrypoint,
        only = "manager",
        live_update = [
            sync(context + "/.tiltbuild/manager", "/manager"),
            run("sh /restart.sh"),
        ],
    )

    # Copy all the substitutions from the user's tilt-settings.json into the environment. Otherwise, the substitutions
    # are not available and their placeholders will be replaced with the empty string when we call kustomize +
    # envsubst below.
    substitutions = settings.get("kustomize_substitutions", {})
    os.environ.update(substitutions)

    # Apply the kustomized yaml for this provider
    yaml = str(kustomizesub(context + "/config"))
    yaml = strip_sec_ctx(yaml)
    k8s_yaml(blob(yaml))

def kustomizesub(folder):
    yaml = local('kustomize build {}'.format(folder), quiet=True)
    return yaml

def strip_sec_ctx(yaml):
    # strip security contexts so tilt's live update keeps working
    # even if there is strict securitycontexts in place for controllers
    output = []
    yamls = decode_yaml_stream(yaml)
    for data in yamls:
        if data.get("kind") == "Deployment":
            spec = data["spec"]["template"]["spec"]
            spec["securityContext"] = {}
            for container in spec.get("containers", []):
                container["securityContext"] = {}
        output.append(str(encode_yaml(data)))

    return "---\n".join(output)

# Users may define their own Tilt customizations in tilt.d. This directory is excluded from git and these files will
# not be checked in to version control.
def include_user_tilt_files():
    user_tiltfiles = listdir("tilt.d")
    for f in user_tiltfiles:
        include(f)

def include_custom_buttons():

    local_resource(
        name = "BareMetalHosts",
        cmd = ["bash", "-c", "echo This is a local resource for BareMetalHosts"],
        auto_init = False,
        trigger_mode = TRIGGER_MODE_MANUAL,
    )

    cmd_button(
        'BareMetalHosts:add_new_bmh',
        argv=['sh', '-c', 'tools/bmh_test/create_bmh.sh $NAME $VBMC_PORT $CONSUMER $CONSUMER_NAMESPACE' ],
        resource = "BareMetalHosts",
        icon_name='add_box',
        text='Add New baremetalhost',
        inputs=[
            text_input('NAME', '"bmh-test-" is automatically added to the begining of the name. This naming convention is later used to clean the local testing environment.'),
            text_input('VBMC_PORT'),
            text_input('CONSUMER'),
            text_input('CONSUMER_NAMESPACE'),
        ],
    )


##############################
# Actual work happens here
##############################

validate_auth()

include_user_tilt_files()

include_custom_buttons()

load_provider_tiltfiles(["."])
local("make tools/bin/kustomize")
enable_provider("metal3-bmo")
