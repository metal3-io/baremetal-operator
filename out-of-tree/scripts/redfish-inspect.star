"""Out of band inspection provisioner driven entirely by Redfish.

Inspects a BareMetalHost through the BMC without booting a ramdisk, as
specified in docs/redfish-inspection.md, and controls power, health and
virtual media over the same session. There is no Ironic dependency, so a
host reaches Available but cannot be Provisioned. OS deployment, cleaning,
RAID and firmware settings all report unsupported.

Lint:
  buildifier --type=default --lint=warn --mode=check \
    --warnings=confusing-name,duplicated-name,function-docstring,\
  function-docstring-header,integer-division,list-append,module-docstring,\
  name-conventions,no-effect,print,redefined-variable,return-value,\
  string-iteration,uninitialized,unreachable,unused-variable \
    scripts/redfish-inspect.star
"""

# HostData dict keys (JSON serialized from the Go HostData struct).
HOST_BMC_ADDRESS = "BMCAddress"
HOST_BMC_CREDS = "BMCCredentials"
HOST_CRED_USER = "Username"
HOST_CRED_PASS = "Password"
HOST_DISABLE_CERT_VERIFY = "DisableCertificateVerification"
HOST_PROVISIONER_ID = "ProvisionerID"
HOST_OBJECT_META = "ObjectMeta"

# BMC address schemes backed by Redfish, mirroring the BMO bmc package. Each
# also accepts a +http or +https transport suffix.
REDFISH_SCHEMES = [
    "idrac-redfish",
    "idrac-virtualmedia",
    "ilo5-redfish",
    "ilo5-virtualmedia",
    "redfish",
    "redfish-uefihttp",
    "redfish-virtualmedia",
]

# Transport used when the BMC scheme carries no suffix, matching BMO.
DEFAULT_TRANSPORT = "https"
TRANSPORTS = ["http", "https"]

# Inventory sections redfish_inventory can collect.
INVENTORY_SECTIONS = [
    "systemVendor",
    "firmware",
    "ramMebibytes",
    "cpu",
    "nics",
    "storage",
]

# Sections an Ironic style inspection treats as mandatory. A gap here is
# reported to the operator but does not fail the inspection.
REQUIRED_SECTIONS = ["ramMebibytes", "cpu", "nics", "storage"]

# InspectionMode values (metal3api.InspectionMode). This script always does fast.
INSPECTION_MODE_AGENT = "agent"
INSPECTION_MODE_FAST = "fast"

# Redfish PowerState values that mean a transition is already under way.
POWERING_ON = "PoweringOn"
POWERING_OFF = "PoweringOff"

# Health strings the controller understands (provisioner.Health*).
HEALTH_OK = "OK"
HEALTH_CRITICAL = "Critical"

# Requeue delays (seconds).
REQUEUE_DELAY = 10
POWER_REQUEUE_DELAY = 15
INSPECT_REQUEUE_DELAY = 30

# Forces insecure transport for every BMC, on top of the per host
# disableCertificateVerification field.
FORCE_INSECURE = getenv("REDFISH_INSECURE").lower() == "true"

def parse_fields(raw):
    """Parse the comma separated section list, empty meaning every section."""
    out = []
    for part in raw.split(","):
        name = part.strip()
        if not name:
            continue
        if name not in INVENTORY_SECTIONS:
            fail("unknown inventory section " + repr(name) + ", valid names are " + repr(INVENTORY_SECTIONS))
        out.append(name)
    return out

# Restricts collection to the named sections, which also skips their Redfish
# calls. Empty collects everything.
INVENTORY_FIELDS = parse_fields(getenv("REDFISH_INSPECT_FIELDS"))

def wanted(section):
    """Report whether a section is collected under the configured selection."""
    if not INVENTORY_FIELDS:
        return True
    return section in INVENTORY_FIELDS

def split_scheme(addr):
    """Split a BMC address into its scheme and the remainder after the separator."""
    end = addr.find("://")
    if end < 0:
        fail("BMC address " + repr(addr) + " has no scheme, one of " + repr(REDFISH_SCHEMES) + " is required")
    return addr[:end], addr[end + 3:]

def parse_bmc_address(addr):
    """Map a BMC address to the Redfish transport and host authority."""
    scheme, rest = split_scheme(addr)
    parts = scheme.split("+")
    if parts[0] not in REDFISH_SCHEMES:
        fail("unsupported BMC scheme " + repr(scheme) + ", one of " + repr(REDFISH_SCHEMES) + " is required")

    transport = parts[1] if len(parts) > 1 else DEFAULT_TRANSPORT
    if transport not in TRANSPORTS:
        fail("unsupported BMC transport " + repr(transport) + " in scheme " + repr(scheme))

    # The endpoint carries no path, the builtin appends every @odata.id itself.
    slash = rest.find("/")
    authority = rest if slash < 0 else rest[:slash]
    if not authority:
        fail("BMC address " + repr(addr) + " has no host")
    return transport, authority

def redfish_endpoint(host):
    """Return the Redfish service endpoint, scheme and authority with no path."""
    addr = host.get(HOST_BMC_ADDRESS, "") if host else ""
    if not addr:
        fail("BMC address is empty, a Redfish address is required")
    transport, authority = parse_bmc_address(addr)
    return transport + "://" + authority

def require_redfish_bmc(host):
    """Abort unless the host carries a usable Redfish BMC address."""
    redfish_endpoint(host)

def conn(host):
    """Return the endpoint, username, password and insecure flag for the host."""
    creds = host.get(HOST_BMC_CREDS, {}) or {}
    insecure = FORCE_INSECURE or bool(host.get(HOST_DISABLE_CERT_VERIFY, False))
    return (
        redfish_endpoint(host),
        creds.get(HOST_CRED_USER, ""),
        creds.get(HOST_CRED_PASS, ""),
        insecure,
    )

def power_status(host):
    """Read the system power state over Redfish."""
    endpoint, user, password, insecure = conn(host)
    return redfish_power_status(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )

def collect_inventory(host):
    """Collect the configured inventory sections, shaped like HardwareDetails."""
    endpoint, user, password, insecure = conn(host)
    if INVENTORY_FIELDS:
        return redfish_inventory(
            endpoint = endpoint,
            username = user,
            password = password,
            insecure = insecure,
            fields = INVENTORY_FIELDS,
        )
    return redfish_inventory(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )

def inspection_gaps(details):
    """Return the requested required sections the BMC did not serve."""
    gaps = []
    for section in REQUIRED_SECTIONS:
        if not wanted(section):
            continue
        if section == "cpu":
            if not details.get("cpu", {}).get("count"):
                gaps.append("cpu.count")
            continue
        if not details.get(section):
            gaps.append(section)
    return gaps

def warn_agent_inspection(data):
    """Note that a request for agent inspection is served out of band instead."""
    mode = data.get("InspectionMode", "") if data else ""
    if mode == INSPECTION_MODE_AGENT:
        log_error(
            "inspect: agent inspection requested, serving out of band instead",
            requested = mode,
            served = INSPECTION_MODE_FAST,
        )
        publish_event(
            "InspectionModeIgnored",
            "Agent inspection is unavailable, inspecting out of band over Redfish",
        )

def warn_provisioning_requested(data):
    """Note early that an image or custom deploy will never be acted on."""
    if not data:
        return
    if data.get("CurrentImage") or data.get("HasCustomDeploy"):
        log_error("register: provisioning is not supported by this script")

def unsupported(method, detail):
    """Build a result reporting that a method is out of scope for this script."""
    return {"error": method + ": " + detail + " is not supported by the redfish-inspect script"}

# Below required by provisioner interface.

def has_capacity(host):
    """Report capacity, out of band inspection contends for nothing."""
    require_redfish_bmc(host)
    return {"has_capacity": True}

def register(host, data, creds_changed, _restart_on_failure):
    """Validate the BMC address, probe Redfish once, and claim the host by UID."""
    require_redfish_bmc(host)
    warn_provisioning_requested(data)

    known = host.get(HOST_PROVISIONER_ID, "")
    meta = host.get(HOST_OBJECT_META, {}) or {}
    prov_id = known or meta.get("uid", "")
    if not prov_id:
        return {
            "dirty": True,
            "requeue_after_seconds": REQUEUE_DELAY,
            "error": "register: BareMetalHost has no UID to claim",
        }

    # Probe only when first claiming the host or when the credentials rotated,
    # so steady state reconciles cost no BMC round trip.
    if not known or creds_changed:
        endpoint, user, password, insecure = conn(host)
        system = redfish_get_system(
            endpoint = endpoint,
            username = user,
            password = password,
            insecure = insecure,
        )
        log_info(
            "register: Redfish reachable",
            endpoint = endpoint,
            manufacturer = system.get("manufacturer", ""),
            model = system.get("model", ""),
        )
        publish_event("Registered", "Registered host over Redfish")

    return {"provID": prov_id}

def preprovisioning_image_formats(host):
    """Report no preprovisioning image, out of band inspection boots nothing."""
    require_redfish_bmc(host)
    return None

def inspect_hardware(host, data, _restart_on_failure, _refresh, _force_reboot):
    """Collect hardware details over Redfish in one pass, with no ramdisk boot."""
    require_redfish_bmc(host)
    warn_agent_inspection(data)

    details = collect_inventory(host)
    if not details:
        return {
            "dirty": True,
            "requeue_after_seconds": INSPECT_REQUEUE_DELAY,
            "error": "inspect: Redfish served no usable inventory",
        }

    # A partial inventory is still recorded, the gaps are what an operator needs
    # to know about their BMC. See the degradation table in the Redfish doc.
    gaps = inspection_gaps(details)
    if gaps:
        missing = ", ".join(gaps)
        log_error("inspect: Redfish inventory is incomplete", missing = missing)
        publish_event("InspectionIncomplete", "Redfish did not serve " + missing)

    log_info("inspect: collected out of band inventory", sections = ", ".join(sorted(details.keys())))
    publish_event("InspectionComplete", "Out of band inspection completed over Redfish")
    return {"hardwareDetails": details}

def update_hardware_state(host):
    """Read the power state over Redfish."""
    require_redfish_bmc(host)
    status = power_status(host)
    return {"PoweredOn": status.get("power_on", False)}

def adopt(host, _data, _restart_on_failure):
    """Adopt the host, nothing is held out of band so this always succeeds."""
    require_redfish_bmc(host)
    return {}

def prepare(host, data, _unprepared, _restart_on_failure):
    """Report nothing to prepare, and reject RAID which needs a deploy agent."""
    require_redfish_bmc(host)
    for key in ("TargetRAIDConfig", "ActualRAIDConfig"):
        cfg = data.get(key) if data else None
        if not cfg:
            continue
        if cfg.get("hardwareRAIDVolumes") or cfg.get("softwareRAIDVolumes"):
            fail("RAID is not supported, remove " + key + " from the BareMetalHost spec")
    return {"started": False}

def service(host, _data, _unprepared, _restart_on_failure):
    """Report nothing to service, servicing needs a deploy agent."""
    require_redfish_bmc(host)
    return {"started": False}

def provision(host, data, _force_reboot):
    """Reject provisioning, this script inspects and controls power only."""
    require_redfish_bmc(host)
    image = data.get("Image", {}) if data else {}
    log_error("provision: unsupported request", image = image.get("url", ""))
    return unsupported("provision", "OS deployment")

# Teardown never touches the BMC, so it deliberately skips address validation.
# A host with an unusable BMC address must still be deletable.

def deprovision(_host, _restart_on_failure, _automated_cleaning_mode):
    """Report deprovisioning complete, nothing was ever deployed or cleaned."""
    return {}

def delete(_host):
    """Release the host, no external record is kept to remove."""
    return {}

def detach(host, _force):
    """Detach the host, which is the same as delete for this script."""
    return delete(host)

def power_on(host, _force):
    """Power the system on over Redfish."""
    require_redfish_bmc(host)
    status = power_status(host)
    if status.get("power_on", False):
        return {}

    state = status.get("power_state", "")
    if state == POWERING_ON:
        log_debug("power_on: already powering on")
        return {"dirty": True, "requeue_after_seconds": POWER_REQUEUE_DELAY}

    endpoint, user, password, insecure = conn(host)
    log_info("power_on: powering on", power_state = state)
    redfish_power_on(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )
    publish_event("PowerOn", "Host powered on over Redfish")
    return {"dirty": True, "requeue_after_seconds": POWER_REQUEUE_DELAY}

def power_off(host, reboot_mode, force, _automated_cleaning_mode):
    """Power the system off over Redfish, gracefully unless forced."""
    require_redfish_bmc(host)
    status = power_status(host)
    if not status.get("power_on", True):
        return {}

    state = status.get("power_state", "")
    if state == POWERING_OFF:
        log_debug("power_off: already powering off")
        return {"dirty": True, "requeue_after_seconds": POWER_REQUEUE_DELAY}

    endpoint, user, password, insecure = conn(host)
    soft = reboot_mode == "soft" and not force
    if soft:
        log_info("power_off: graceful shutdown", power_state = state)
        redfish_power_soft(
            endpoint = endpoint,
            username = user,
            password = password,
            insecure = insecure,
        )
    else:
        log_info("power_off: forced off", power_state = state)
        redfish_power_off(
            endpoint = endpoint,
            username = user,
            password = password,
            insecure = insecure,
        )

    publish_event("PowerOff", "Host powered off over Redfish")
    return {"dirty": True, "requeue_after_seconds": POWER_REQUEUE_DELAY}

def get_firmware_settings(host, _include_schema):
    """Report no firmware settings, reading the BIOS attributes is out of scope."""
    require_redfish_bmc(host)
    return None

def get_firmware_components(host):
    """Report the BIOS version, the one component this collector reads."""
    require_redfish_bmc(host)
    endpoint, user, password, insecure = conn(host)
    firmware = redfish_get_firmware(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )

    version = firmware.get("bios_version", "")
    if not version:
        return None

    # Out of band there is no flashing history, so the initial version is the
    # version being read now.
    return [{
        "component": "bios",
        "initialVersion": version,
        "currentVersion": version,
    }]

def add_bmc_event_subscription(host, _subscription):
    """Reject event subscriptions, the plugin exposes no Redfish event API."""
    require_redfish_bmc(host)
    return unsupported("add_bmc_event_subscription", "BMC event subscriptions")

def remove_bmc_event_subscription(host, _subscription):
    """Report the subscription gone, none was ever created."""
    require_redfish_bmc(host)
    return {}

def get_data_image_status(host):
    """Report whether virtual media currently holds an image."""
    require_redfish_bmc(host)
    endpoint, user, password, insecure = conn(host)
    status = redfish_media_status(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )
    return {"attached": status.get("inserted", False)}

def attach_data_image(host, url):
    """Insert a data image into virtual media over Redfish."""
    require_redfish_bmc(host)
    endpoint, user, password, insecure = conn(host)
    log_info("attach_data_image: inserting media", image = url)
    redfish_insert_media(
        endpoint = endpoint,
        username = user,
        password = password,
        image = url,
        insecure = insecure,
    )

def detach_data_image(host):
    """Eject the mounted data image over Redfish."""
    require_redfish_bmc(host)
    endpoint, user, password, insecure = conn(host)
    log_info("detach_data_image: ejecting media")
    redfish_eject_media(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )

def has_power_failure(host):
    """Report no power fault, the Redfish power state models none."""
    require_redfish_bmc(host)
    return False

def get_health(host):
    """Map the Redfish system health rollup to a controller health string."""
    require_redfish_bmc(host)
    endpoint, user, password, insecure = conn(host)
    healthy = redfish_is_healthy(
        endpoint = endpoint,
        username = user,
        password = password,
        insecure = insecure,
    )
    return HEALTH_OK if healthy else HEALTH_CRITICAL
