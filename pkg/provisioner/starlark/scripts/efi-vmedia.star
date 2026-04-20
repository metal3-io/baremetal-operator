"""Ironic provisioner for redfish-virtualmedia + UEFI, custom-deploy or direct qcow2.

Lint:
  buildifier --type=default --lint=warn --mode=check \
    --warnings=confusing-name,duplicated-name,function-docstring,\
  function-docstring-header,integer-division,list-append,module-docstring,\
  name-conventions,no-effect,print,redefined-variable,return-value,\
  string-iteration,uninitialized,unreachable,unused-variable \
    pkg/provisioner/starlark/scripts/efi-vmedia.star
"""
API_VERSION = "1.89"
API_VERSION_HEALTH = "1.109"

# HostData dict keys (JSON-serialized from Go HostData struct)
HOST_BMC_ADDRESS = "BMCAddress"
HOST_BMC_CREDS = "BMCCredentials"
HOST_CRED_USER = "Username"
HOST_CRED_PASS = "Password"
HOST_PROVISIONER_ID = "ProvisionerID"
HOST_BOOT_MAC = "BootMACAddress"

# Only redfish-virtualmedia BMCs and UEFI boot are supported; enforced at entry.
SUPPORTED_BMC_SCHEME = "redfish-virtualmedia"
SUPPORTED_BOOT_MODE = "UEFI"

# Accepted Image.checksumType values (empty / "auto" → Ironic detects).
SUPPORTED_CHECKSUM_TYPES = ["", "auto", "md5", "sha256", "sha512"]

# BMO RootDeviceHints JSON tag -> Ironic root_device key (only serial/wwn supported).
SUPPORTED_ROOT_DEVICE_HINTS = {
    "serialNumber": "serial",
    "wwn": "wwn",
}

# Provision states (gophercloud v2.12.0 constants)
ENROLL = "enroll"
VERIFYING = "verifying"
MANAGEABLE = "manageable"
AVAILABLE = "available"
ACTIVE = "active"
DEPLOY_WAIT = "wait call-back"
DEPLOYING = "deploying"
DEPLOY_FAIL = "deploy failed"
DELETING = "deleting"
CLEANING = "cleaning"
CLEAN_WAIT = "clean wait"
CLEAN_FAIL = "clean failed"
INSPECTING = "inspecting"
INSPECT_WAIT = "inspect wait"
INSPECT_FAIL = "inspect failed"
ADOPTING = "adopting"
ADOPT_FAIL = "adopt failed"
ERROR = "error"

# BMH provisioning state strings (metal3api.ProvisioningState).
BMH_STATE_DEPROVISIONING = "deprovisioning"
BMH_STATE_DELETING = "deleting"

# Power states
POWER_ON = "power on"
POWER_OFF = "power off"
SOFT_POWER_OFF = "soft power off"

# Requeue delays (seconds)
REQUEUE_DELAY = 10
INSPECT_REQUEUE_DELAY = 15
SOFT_POWER_OFF_TIMEOUT = 180

# HTTP timeout for Ironic requests (seconds)
HTTP_TIMEOUT = 60

# Custom deploy priority
CUSTOM_DEPLOY_PRIORITY = 80

# Ironic connection settings
IRONIC_ENDPOINT = getenv("IRONIC_ENDPOINT").rstrip("/").removesuffix("/v1")
IRONIC_INSECURE = getenv("IRONIC_INSECURE").lower() == "true"
IRONIC_VERIFY_CERTS = not IRONIC_INSECURE
AUTH_ROOT = getenv("METAL3AUTH_ROOT_DIR") or "/opt/metal3/auth"

# IPA deploy images (needed for inspection and deployment)
DEPLOY_KERNEL_URL = getenv("DEPLOY_KERNEL_URL")
DEPLOY_RAMDISK_URL = getenv("DEPLOY_RAMDISK_URL")
IRONIC_USERNAME = read_file(AUTH_ROOT + "/ironic/username")
IRONIC_PASSWORD = read_file(AUTH_ROOT + "/ironic/password")

def make_url(path):
    """Build full Ironic API URL."""
    base = IRONIC_ENDPOINT.rstrip("/")
    if not path.startswith("/"):
        path = "/" + path
    return base + path

def http_get(path, api_version = API_VERSION, ignore_statuses = []):
    """HTTP GET returning (parsed_json, status_code); ignore_statuses silences expected >=400s."""
    resp_body, status, _ = http_request_raw(
        "GET",
        make_url(path),
        IRONIC_USERNAME,
        IRONIC_PASSWORD,
        IRONIC_VERIFY_CERTS,
        HTTP_TIMEOUT,
        "",
        {"X-OpenStack-Ironic-API-Version": api_version, "Content-Type": "application/json"},
    )
    if status >= 200 and status < 300 and resp_body:
        return json_decode(resp_body), status
    if status >= 400 and resp_body and status not in ignore_statuses:
        log_error("HTTP GET " + path + " failed", status = status, response = resp_body)
    return None, status

def http_patch(path, ops, ignore_statuses = []):
    """HTTP PATCH with JSON-Patch body, returning (parsed_json, status_code)."""
    body = json_encode(ops)
    resp_body, status, _ = http_request_raw(
        "PATCH",
        make_url(path),
        IRONIC_USERNAME,
        IRONIC_PASSWORD,
        IRONIC_VERIFY_CERTS,
        HTTP_TIMEOUT,
        body,
        {"X-OpenStack-Ironic-API-Version": API_VERSION, "Content-Type": "application/json"},
    )
    if status >= 200 and status < 300 and resp_body:
        return json_decode(resp_body), status
    if status >= 400 and resp_body and status not in ignore_statuses:
        log_error("HTTP PATCH " + path + " failed", status = status, response = resp_body)
    return None, status

def http_post(path, body_dict, ignore_statuses = []):
    """HTTP POST returning (parsed_json, status_code)."""
    body = json_encode(body_dict) if body_dict else ""
    resp_body, status, _ = http_request_raw(
        "POST",
        make_url(path),
        IRONIC_USERNAME,
        IRONIC_PASSWORD,
        IRONIC_VERIFY_CERTS,
        HTTP_TIMEOUT,
        body,
        {"X-OpenStack-Ironic-API-Version": API_VERSION, "Content-Type": "application/json"},
    )
    if status >= 200 and status < 300 and resp_body:
        return json_decode(resp_body), status
    if status >= 400 and resp_body and status not in ignore_statuses:
        log_error("HTTP POST " + path + " failed", status = status, response = resp_body)
    return None, status

def http_put(path, body_dict, ignore_statuses = []):
    """HTTP PUT returning (parsed_json, status_code)."""
    body = json_encode(body_dict) if body_dict else ""
    resp_body, status, _ = http_request_raw(
        "PUT",
        make_url(path),
        IRONIC_USERNAME,
        IRONIC_PASSWORD,
        IRONIC_VERIFY_CERTS,
        HTTP_TIMEOUT,
        body,
        {"X-OpenStack-Ironic-API-Version": API_VERSION, "Content-Type": "application/json"},
    )
    if status >= 200 and status < 300 and resp_body:
        return json_decode(resp_body), status
    if status >= 400 and resp_body and status not in ignore_statuses:
        log_error("HTTP PUT " + path + " failed", status = status, response = resp_body)
    return None, status

def http_delete(path, query = ""):
    """HTTP DELETE returning status_code."""
    full_path = path
    if query:
        full_path = path + "?" + query
    _, status, _ = http_request_raw(
        "DELETE",
        make_url(full_path),
        IRONIC_USERNAME,
        IRONIC_PASSWORD,
        IRONIC_VERIFY_CERTS,
        HTTP_TIMEOUT,
        "",
        {"X-OpenStack-Ironic-API-Version": API_VERSION, "Content-Type": "application/json"},
    )
    return status

def change_prov_state(prov_id, target, configdrive = None, deploy_steps = None, clean_steps = None, service_steps = None):
    """PUT /v1/nodes/{id}/states/provision."""
    body = {"target": target}
    if configdrive != None:
        body["configdrive"] = configdrive
    if deploy_steps != None:
        body["deploy_steps"] = deploy_steps
    if clean_steps != None:
        body["clean_steps"] = clean_steps
    if service_steps != None:
        body["service_steps"] = service_steps
    return http_put("/v1/nodes/" + prov_id + "/states/provision", body)

def change_power_state(prov_id, target, timeout = 0):
    """PUT /v1/nodes/{id}/states/power."""
    body = {"target": target}
    if timeout > 0:
        body["timeout"] = timeout
    return http_put("/v1/nodes/" + prov_id + "/states/power", body)

def set_maintenance(prov_id, reason = ""):
    """PUT /v1/nodes/{id}/maintenance."""
    body = {}
    if reason:
        body["reason"] = reason
    return http_put("/v1/nodes/" + prov_id + "/maintenance", body)

def unset_maintenance(prov_id):
    """DELETE /v1/nodes/{id}/maintenance."""
    return http_delete("/v1/nodes/" + prov_id + "/maintenance")

def patch_op(op_type, path, value = None):
    """Build a single JSON-Patch operation; None -> JSON null (clears the field)."""
    return {"op": op_type, "path": path, "value": value}

def parse_hardware_details(inv):
    """Convert Ironic inventory to BMO HardwareDetails dict."""
    inventory = inv.get("inventory", {})
    details = {}

    # System vendor
    sv = inventory.get("system_vendor", {})
    if sv:
        details["systemVendor"] = {
            "manufacturer": sv.get("manufacturer", ""),
            "productName": sv.get("product_name", ""),
            "serialNumber": sv.get("serial_number", ""),
        }

    # Firmware
    bios = inventory.get("bios", {})
    if bios:
        details["firmware"] = {
            "bios": {
                "vendor": bios.get("vendor", ""),
                "version": bios.get("version", ""),
                "date": bios.get("date", ""),
            },
        }

    # RAM
    mem = inventory.get("memory", {})
    if mem:
        details["ramMebibytes"] = mem.get("physical_mb", 0)

    # CPU
    cpu = inventory.get("cpu", {})
    if cpu:
        details["cpu"] = {
            "arch": cpu.get("architecture", ""),
            "model": cpu.get("model_name", ""),
            "count": cpu.get("count", 0),
            "flags": cpu.get("flags", []),
        }
        freq = cpu.get("frequency", "")
        if freq:
            details["cpu"]["clockMegahertz"] = parse_freq(freq)

    # NICs
    nics = []
    for iface in inventory.get("interfaces", []):
        if not iface.get("mac_address"):
            continue
        nic = {
            "name": iface.get("name", ""),
            "mac": iface.get("mac_address", ""),
            "ip": iface.get("ipv4_address", ""),
            "speedGbps": 0,
            "model": iface.get("vendor", ""),
        }
        speed = iface.get("speed_mbps")
        if speed and speed > 0:
            nic["speedGbps"] = speed // 1000
        if iface.get("product"):
            nic["model"] = iface.get("vendor", "") + " " + iface.get("product", "")
        nics.append(nic)
    details["nics"] = nics

    # Storage
    disks = []
    for d in inventory.get("disks", []):
        disk = {
            "name": d.get("name", ""),
            "sizeBytes": d.get("size", 0),
            "rotational": d.get("rotational", False),
            "serialNumber": d.get("serial", ""),
            "model": d.get("model", ""),
            "vendor": d.get("vendor", ""),
            "wwn": d.get("wwn", ""),
            "hctl": d.get("hctl", ""),
        }
        if d.get("wwn_with_extension"):
            disk["wwnWithExtension"] = d["wwn_with_extension"]
        if d.get("wwn_vendor_extension"):
            disk["wwnVendorExtension"] = d["wwn_vendor_extension"]
        if d.get("by_path"):
            disk["byPath"] = d["by_path"]
        disks.append(disk)
    details["storage"] = disks

    # Hostname
    hostname = inventory.get("hostname", "")
    if hostname:
        details["hostname"] = hostname

    return details

def parse_freq(freq_str):
    """Parses frequency string to MHz float."""
    if not freq_str:
        return 0.0
    s = freq_str.strip().lower()
    if s.endswith("ghz"):
        return float(s.removesuffix("ghz").strip()) * 1000.0
    if s.endswith("mhz"):
        return float(s.removesuffix("mhz").strip())
    return float(s)

def parse_bmc_address(bmc_addr):
    """Parses redfish-virtualmedia://host:port/path into driver_info fields."""
    addr = bmc_addr

    # Strip scheme
    scheme_end = addr.find("://")
    if scheme_end >= 0:
        addr = addr[scheme_end + 3:]

    # Split host:port from path
    slash = addr.find("/")
    if slash >= 0:
        host_port = addr[:slash]
        system_id = addr[slash:]
    else:
        host_port = addr
        system_id = ""
    return {
        "redfish_address": "https://" + host_port,
        "redfish_system_id": system_id,
    }

def resolve_ironic_network_data():
    """Resolve Ironic node.network_data for IPA ramdisk: preprov secret first, then Spec.NetworkData."""
    nd_raw = read_host_secret("preprovisioningNetworkData")
    if not nd_raw:
        nd_raw = read_host_secret("networkData")
    if not nd_raw:
        return None
    nd = yaml_decode(nd_raw)

    # Ironic schema requires network_id on each network entry; CAPM3 omits it.
    for net in nd.get("networks", []):
        if "network_id" not in net:
            net["network_id"] = net.get("id", "")
    return nd

def create_node(bmc_addr, bmc_user, bmc_password, boot_mac, data):
    """Creates a new node in Ironic via POST /v1/nodes."""
    bmc = parse_bmc_address(bmc_addr)
    boot_mode = data.get("BootMode", "UEFI")
    cpu_arch = data.get("CPUArchitecture", "x86_64")
    body = {
        "driver": "redfish",
        "boot_interface": "redfish-virtual-media",
        "inspect_interface": "agent",
        "bios_interface": "redfish",
        "management_interface": "redfish",
        "power_interface": "redfish",
        "raid_interface": "redfish",
        "firmware_interface": "redfish",
        "vendor_interface": "no-vendor",
        "automated_clean": data.get("AutomatedCleaningMode", "") != "disabled",
        "driver_info": {
            "redfish_address": bmc["redfish_address"],
            "redfish_system_id": bmc["redfish_system_id"],
            "redfish_username": bmc_user,
            "redfish_password": bmc_password,
            "redfish_verify_ca": False,
            "deploy_kernel": DEPLOY_KERNEL_URL,
            "deploy_ramdisk": DEPLOY_RAMDISK_URL,
        },
        "properties": {
            "capabilities": "boot_mode:" + boot_mode.lower(),
            "cpu_arch": cpu_arch,
        },
    }
    nd = resolve_ironic_network_data()
    if nd:
        body["network_data"] = nd
    node, status = http_post("/v1/nodes", body)
    if status == 409:
        return None, status
    if status < 200 or status >= 300:
        return None, status

    # Create port so IPA can look up the node by MAC during inspection
    if boot_mac and node:
        port_body = {
            "node_uuid": node.get("uuid", ""),
            "address": boot_mac,
            "pxe_enabled": True,
        }
        http_post("/v1/ports", port_body)

    return node, status

def build_register_patch(node, data, _creds_changed):
    """Builds JSON-Patch operations for node configuration during register."""
    ops = []
    current_props = node.get("properties", {})

    # Boot mode capabilities
    boot_mode = data.get("BootMode", "")
    if boot_mode:
        cap_str = "boot_mode:" + boot_mode.lower()
        current_cap = current_props.get("capabilities", "")
        if cap_str != current_cap:
            ops.append(patch_op("add", "/properties/capabilities", cap_str))

    # CPU architecture
    cpu_arch = data.get("CPUArchitecture", "")
    if cpu_arch and cpu_arch != current_props.get("cpu_arch", ""):
        ops.append(patch_op("add", "/properties/cpu_arch", cpu_arch))

    # Automated cleaning mode
    acm = data.get("AutomatedCleaningMode", "")
    if acm:
        want_clean = acm != "disabled"
        current_clean = node.get("automated_clean")
        if current_clean == None or current_clean != want_clean:
            ops.append(patch_op("add", "/automated_clean", want_clean))

    # DisablePowerOff
    disable_po = data.get("DisablePowerOff", False)
    if disable_po != node.get("disable_power_off", False):
        ops.append(patch_op("add", "/disable_power_off", disable_po))

    # IPA deploy images
    current_di = node.get("driver_info", {})
    if DEPLOY_KERNEL_URL and current_di.get("deploy_kernel") != DEPLOY_KERNEL_URL:
        ops.append(patch_op("add", "/driver_info/deploy_kernel", DEPLOY_KERNEL_URL))
    if DEPLOY_RAMDISK_URL and current_di.get("deploy_ramdisk") != DEPLOY_RAMDISK_URL:
        ops.append(patch_op("add", "/driver_info/deploy_ramdisk", DEPLOY_RAMDISK_URL))

    # Network data for IPA (so it can reach Ironic callback endpoint)
    nd = resolve_ironic_network_data()
    if nd and nd != node.get("network_data"):
        ops.append(patch_op("add", "/network_data", nd))

    return ops

def require_redfish_virtualmedia(host):
    """Abort unless BMC address uses the redfish-virtualmedia scheme."""
    addr = host.get(HOST_BMC_ADDRESS, "") if host else ""
    if not addr:
        fail("BMC address is empty; " + SUPPORTED_BMC_SCHEME + " is required")
    scheme_end = addr.find("://")
    if scheme_end < 0:
        fail("BMC address " + repr(addr) + " has no scheme; " + SUPPORTED_BMC_SCHEME + " is required")
    scheme = addr[:scheme_end]

    # redfish-virtualmedia+http / +https normalize to the same Ironic driver.
    base = scheme.split("+")[0]
    if base != SUPPORTED_BMC_SCHEME:
        fail("unsupported BMC scheme " + repr(scheme) + "; only " + repr(SUPPORTED_BMC_SCHEME) + " is supported")

def validate_boot_mode(data):
    """Reject BootMode values other than UEFI (empty means default -> UEFI)."""
    mode = data.get("BootMode", "") if data else ""
    if mode and mode != SUPPORTED_BOOT_MODE:
        fail("unsupported BootMode " + repr(mode) + "; only " + repr(SUPPORTED_BOOT_MODE) + " (or empty default) is supported")

def translate_root_device_hints(hints):
    """Translate BMO RootDeviceHints to Ironic's root_device shape; fail() on unsupported keys."""
    if not hints:
        return {}
    out = {}
    unsupported = []
    for k, v in hints.items():
        ironic_key = SUPPORTED_ROOT_DEVICE_HINTS.get(k)
        if ironic_key:
            out[ironic_key] = v
        else:
            unsupported.append(k)
    if unsupported:
        fail("unsupported rootDeviceHints keys " + repr(sorted(unsupported)) +
             "; only " + repr(sorted(SUPPORTED_ROOT_DEVICE_HINTS.keys())) + " are supported")
    return out

def get_image_checksum(image):
    """Return (checksum, checksum_type) mirroring Image.GetChecksum()."""
    if not image:
        fail("image is not provided")
    url = image.get("url", "")
    checksum = image.get("checksum", "")
    if url.startswith("oci://") and not checksum:
        return "", ""
    if not checksum:
        fail("checksum is required for normal images")
    ct = image.get("checksumType", "")
    if ct not in SUPPORTED_CHECKSUM_TYPES:
        fail("unknown checksumType " + repr(ct) + "; supported: " + repr(SUPPORTED_CHECKSUM_TYPES))
    if ct in ["", "auto"]:
        return checksum, ""
    return checksum, ct

def require_no_raid(data):
    """Abort if the BMH has RAID configured."""
    if not data:
        return
    for key in ("TargetRAIDConfig", "ActualRAIDConfig"):
        cfg = data.get(key)
        if not cfg:
            continue
        if cfg.get("hardwareRAIDVolumes") or cfg.get("softwareRAIDVolumes"):
            fail("RAID is not supported; remove " + key + " from the BareMetalHost spec")

def build_configdrive(host, data):
    """Build configdrive mirroring pkg/provisioner/ironic/ironic.go getConfigDrive."""
    hc = data.get("HostConfig", {}) or {}
    cd = {}

    user_data = hc.get("userData", "")
    if user_data:
        cd["user_data"] = user_data

    network_data_raw = hc.get("networkData", "")
    if network_data_raw:
        cd["network_data"] = yaml_decode(network_data_raw)

    om = host.get("ObjectMeta", {}) or {}
    name = om.get("name", "")
    md = {
        "uuid": om.get("uid", ""),
        "metal3-namespace": om.get("namespace", ""),
        "metal3-name": name,
        "local-hostname": name,
        "local_hostname": name,
        "name": name,
    }
    meta_data_raw = hc.get("metaData", "")
    if meta_data_raw:
        md.update(yaml_decode(meta_data_raw))
    cd["meta_data"] = md

    return cd

# Below required by provisioner interface.

def try_init(host):
    """Checks if Ironic is reachable and has at least one conductor."""
    require_redfish_virtualmedia(host)
    api, status = http_get("/v1/")
    if status != 200 or not api:
        log_error("try_init: cannot reach Ironic API", status = status)
        return {"ready": False}

    drivers, status = http_get("/v1/drivers")
    if status != 200 or not drivers:
        log_error("try_init: cannot list drivers", status = status)
        return {"ready": False}

    driver_list = drivers.get("drivers", [])
    ready = len(driver_list) > 0
    if ready:
        log_info("ironic ready", api_version = api.get("version", {}).get("version", ""), drivers = len(driver_list))
    return {"ready": ready}

def has_capacity(host):
    """Returns true since virtual-media boot has no shared resource contention."""
    require_redfish_virtualmedia(host)
    return {"has_capacity": True}

def register(host, data, _creds_changed, _restart_on_failure):
    """Find or create Ironic node, configure, transition to manageable."""
    require_redfish_virtualmedia(host)
    validate_boot_mode(data)
    bmc_addr = host[HOST_BMC_ADDRESS]
    bmc_user = host[HOST_BMC_CREDS][HOST_CRED_USER]
    bmc_password = host[HOST_BMC_CREDS][HOST_CRED_PASS]
    prov_id = host[HOST_PROVISIONER_ID]
    boot_mac = host[HOST_BOOT_MAC]
    node = None

    # Find existing node
    if prov_id:
        node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
        if status == 404:
            node = None

    # Create node if not found
    if not node:
        log_info("register: creating new node in ironic")
        node, status = create_node(bmc_addr, bmc_user, bmc_password, boot_mac, data)
        if not node:
            if status == 409:
                return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY, "provID": ""}
            return {"dirty": True, "error": "register: failed to create node, status=" + str(status), "provID": ""}
        publish_event("Registered", "Registered new host")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY, "provID": node.get("uuid", "")}

    node_uuid = node.get("uuid", prov_id)
    state = node.get("provision_state", "")

    # Configure node (update driver_info, properties, etc.)
    ops = build_register_patch(node, data, _creds_changed)
    if ops:
        _, patch_status = http_patch("/v1/nodes/" + node_uuid, ops)
        if patch_status == 409:
            log_info("register: node busy, will retry")
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY, "provID": node_uuid}
        if patch_status < 200 or patch_status >= 300:
            return {"dirty": True, "error": "register: PATCH failed with status " + str(patch_status), "provID": node_uuid}

    # State management
    if state == ENROLL:
        log_info("register: transitioning from enroll to manageable")
        change_prov_state(node_uuid, "manage")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY, "provID": node_uuid}

    if state == VERIFYING:
        log_debug("register: waiting for verification")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY, "provID": node_uuid}

    # Boot-interface validate only; deploy validate needs image_source.
    validate, v_status = http_get("/v1/nodes/" + node_uuid + "/validate")
    if v_status == 200 and validate:
        boot_valid = validate.get("boot", {}).get("result", False)
        if not boot_valid:
            log_error("register: boot validation failed", reason = validate.get("boot", {}).get("reason", "unknown"))

    return {"provID": node_uuid}

def preprovisioning_image_formats(host):
    """Returns supported image formats for preprovisioning image build."""
    require_redfish_virtualmedia(host)
    return ["iso", "initrd"]

def inspect_hardware(host, data, restart_on_failure, refresh, force_reboot):
    """Drive hardware inspection through Ironic."""
    require_redfish_virtualmedia(host)
    validate_boot_mode(data)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id)
    if status != 200 or not node:
        return {"dirty": True, "error": "inspect: cannot get node"}

    state = node.get("provision_state", "")
    last_error = node.get("last_error", "")

    # Handle aborted inspection as refresh
    if state == INSPECT_FAIL and "aborted" in last_error:
        refresh = True
        restart_on_failure = True

    # Transition to manageable if needed
    if state == AVAILABLE:
        log_info("inspect: node is available, transitioning to manageable first")
        change_prov_state(prov_id, "manage")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    # Start inspection
    if state == MANAGEABLE and (refresh or not node.get("inspection_finished_at")):
        boot_mode = data.get("BootMode", "")
        cpu_arch = data.get("CPUArchitecture", "")
        ops = []
        if boot_mode:
            ops.append(patch_op("add", "/properties/capabilities", "boot_mode:" + boot_mode.lower()))
        if cpu_arch:
            ops.append(patch_op("add", "/properties/cpu_arch", cpu_arch))
        if ops:
            http_patch("/v1/nodes/" + prov_id, ops)

        log_info("inspect: starting hardware inspection")
        _, s = change_prov_state(prov_id, "inspect")
        if s == 202:
            publish_event("InspectionStarted", "Hardware inspection started")
            return {"dirty": True, "requeue_after_seconds": INSPECT_REQUEUE_DELAY, "started": True}
        return {"dirty": True, "error": "inspect: failed to start inspection, status=" + str(s)}

    # Retrieve inspection results
    if state == MANAGEABLE and node.get("inspection_finished_at"):
        inv, inv_status = http_get("/v1/nodes/" + prov_id + "/inventory", ignore_statuses = [404])
        if inv_status == 404:
            log_info("inspect: inventory not yet available")
            return {"dirty": True, "requeue_after_seconds": INSPECT_REQUEUE_DELAY}
        if inv_status != 200 or not inv:
            return {"dirty": True, "error": "inspect: failed to get inventory, status=" + str(inv_status)}

        details = parse_hardware_details(inv)
        publish_event("InspectionComplete", "Hardware inspection completed")
        return {"hardwareDetails": details}

    if state == INSPECT_WAIT:
        if force_reboot:
            log_info("inspect: aborting inspection for force reboot")
            change_prov_state(prov_id, "abort")
            return {"dirty": True, "requeue_after_seconds": INSPECT_REQUEUE_DELAY, "started": True}
        return {"dirty": True, "requeue_after_seconds": INSPECT_REQUEUE_DELAY}

    if state == INSPECTING:
        return {"dirty": True, "requeue_after_seconds": INSPECT_REQUEUE_DELAY}

    if state == INSPECT_FAIL:
        if restart_on_failure:
            log_info("inspect: restarting after failure")
            change_prov_state(prov_id, "manage")
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
        return {"dirty": True, "error": "inspect: inspection failed: " + last_error}

    # Unexpected state
    return {"dirty": True, "error": "inspect: unexpected state " + state}

def update_hardware_state(host):
    """Read power state from Ironic."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id)
    if status != 200 or not node:
        return None

    ps = node.get("power_state", "")
    if ps == POWER_ON:
        return {"PoweredOn": True}
    if ps == POWER_OFF:
        return {"PoweredOn": False}
    return None

def adopt(host, data, restart_on_failure):
    """Bring an externally-provisioned node under management; mirrors ironic.go Adopt()."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
    if status != 200 or not node:
        return {"dirty": True, "error": "adopt: cannot get node"}

    state = node.get("provision_state", "")
    bmh_state = data.get("State", "") if data else ""

    if state in [ENROLL, VERIFYING]:
        return {"dirty": True, "error": "adopt: invalid ironic state " + state}

    if state == MANAGEABLE:
        # While deprovisioning, leave Manageable alone — Deprovision will progress it.
        if bmh_state == BMH_STATE_DEPROVISIONING:
            return {}
        _, s = change_prov_state(prov_id, "adopt")
        if s == 409:
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
        if s < 200 or s >= 300:
            return {"dirty": True, "error": "adopt: failed to start adoption, status=" + str(s)}
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == ADOPTING:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == ADOPT_FAIL:
        if restart_on_failure:
            change_prov_state(prov_id, "adopt")
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
        return {"dirty": True, "error": "adopt: adoption failed: " + node.get("last_error", "")}

    if state == ACTIVE:
        # Empty fault means maintenance was set out-of-band; clear it unless we're deleting.
        if node.get("maintenance", False) and node.get("fault", "") == "" and bmh_state != BMH_STATE_DELETING:
            unset_maintenance(prov_id)

    return {}

def prepare(host, data, _unprepared, _restart_on_failure):
    """Reject RAID; sync network_data on the Ironic node for cleaning boots."""
    require_redfish_virtualmedia(host)
    require_no_raid(data)

    prov_id = host[HOST_PROVISIONER_ID]
    if prov_id:
        nd = resolve_ironic_network_data()
        if nd:
            node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
            if status == 200 and node and nd != node.get("network_data"):
                http_patch("/v1/nodes/" + prov_id, [patch_op("add", "/network_data", nd)])

    return {"started": False}

def service(host, _data, _unprepared, _restart_on_failure):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return {"started": False}

def provision(host, data, force_reboot):
    """Provision the node: set instance info, validate, deploy."""
    require_redfish_virtualmedia(host)
    validate_boot_mode(data)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id)
    if status != 200 or not node:
        return {"dirty": True, "error": "provision: cannot get node"}

    # Sync network_data so the deploy ramdisk boots with network config.
    nd = resolve_ironic_network_data()
    if nd and nd != node.get("network_data"):
        http_patch("/v1/nodes/" + prov_id, [patch_op("add", "/network_data", nd)])

    state = node.get("provision_state", "")

    if state == MANAGEABLE:
        log_info("provision: transitioning from manageable to available")
        change_prov_state(prov_id, "provide")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == CLEAN_FAIL:
        log_info("provision: recovering from clean failure")
        if node.get("maintenance", False):
            unset_maintenance(prov_id)
        change_prov_state(prov_id, "manage")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == ACTIVE:
        return {}

    if state == DEPLOY_WAIT and force_reboot:
        log_info("provision: force rebooting during deploy wait")
        change_prov_state(prov_id, "deleted")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == DEPLOY_FAIL:
        return {"dirty": True, "error": "provision: deploy failed: " + node.get("last_error", "")}

    if state in [DEPLOYING, DEPLOY_WAIT, CLEANING, CLEAN_WAIT, DELETING]:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state != AVAILABLE:
        return {"dirty": True, "error": "provision: unexpected state " + state}

    # Deploy mode: CustomDeploy.method → custom-agent, else Image.url → direct.
    image = data.get("Image", {})
    if image and image.get("format", "") == "live-iso":
        fail("live-iso images are not supported")
    custom_deploy = data.get("CustomDeploy")
    has_custom_deploy = custom_deploy != None and custom_deploy.get("method", "") != ""
    image_url = image.get("url", "")
    if not has_custom_deploy and not image_url:
        return {"dirty": True, "error": "provision: requires either Spec.CustomDeploy.Method or Spec.Image.URL"}

    root_hints = translate_root_device_hints(data.get("RootDeviceHints"))
    boot_mode = data.get("BootMode", "")

    ops = []
    if not node.get("instance_uuid", ""):
        ops.append(patch_op("add", "/instance_uuid", prov_id))

    caps = {}
    if boot_mode:
        # Ironic expects "bios" / "uefi" lowercase (BMH CRD uses "UEFI").
        caps["boot_mode"] = boot_mode.lower()
    ops.append(patch_op("add", "/instance_info/capabilities", caps))

    if root_hints:
        ops.append(patch_op("add", "/instance_info/root_device", root_hints))

    if has_custom_deploy:
        # Custom-deploy branch: custom-agent interface, user-defined deploy step.
        if image_url:
            ops.append(patch_op("add", "/instance_info/image_source", image_url))
        ops.append(patch_op("add", "/instance_info/image_os_hash_algo", ""))
        ops.append(patch_op("add", "/instance_info/image_os_hash_value", ""))
        ops.append(patch_op("add", "/deploy_interface", "custom-agent"))
    else:
        # Direct branch: default deploy_interface, checksum shape per Image.GetChecksum.
        ops.append(patch_op("add", "/instance_info/boot_iso", None))
        ops.append(patch_op("add", "/instance_info/image_source", image_url))
        disk_format = image.get("format", "")
        if disk_format:
            ops.append(patch_op("add", "/instance_info/image_disk_format", disk_format))
        checksum, ctype = get_image_checksum(image)
        if checksum == "" and ctype == "":
            ops.append(patch_op("add", "/instance_info/image_checksum", None))
            ops.append(patch_op("add", "/instance_info/image_os_hash_algo", None))
            ops.append(patch_op("add", "/instance_info/image_os_hash_value", None))
        elif ctype == "":
            ops.append(patch_op("add", "/instance_info/image_checksum", checksum))
            ops.append(patch_op("add", "/instance_info/image_os_hash_algo", None))
            ops.append(patch_op("add", "/instance_info/image_os_hash_value", None))
        else:
            ops.append(patch_op("add", "/instance_info/image_checksum", None))
            ops.append(patch_op("add", "/instance_info/image_os_hash_algo", ctype))
            ops.append(patch_op("add", "/instance_info/image_os_hash_value", checksum))

        # Clear stale deploy_interface left by a prior ramdisk / custom-agent attempt.
        cur_di = node.get("deploy_interface", "")
        if cur_di in ("ramdisk", "custom-agent"):
            ops.append(patch_op("add", "/deploy_interface", None))

        # Matches Go's directDeploy default; config override not exposed here.
        ops.append(patch_op("add", "/driver_info/force_persistent_boot_device", "Default"))

    _, patch_status = http_patch("/v1/nodes/" + prov_id, ops)
    if patch_status == 409:
        log_info("provision: node busy during PATCH, will retry")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
    if patch_status < 200 or patch_status >= 300:
        return {"dirty": True, "error": "provision: PATCH failed with status " + str(patch_status)}

    # Validate node configuration
    validate, v_status = http_get("/v1/nodes/" + prov_id + "/validate")
    if v_status == 200 and validate:
        boot_ok = validate.get("boot", {}).get("result", False)
        deploy_ok = validate.get("deploy", {}).get("result", False)
        if not boot_ok:
            log_error("provision: boot validation failed", reason = validate.get("boot", {}).get("reason", "unknown"))
        if not deploy_ok:
            log_error("provision: deploy validation failed", reason = validate.get("deploy", {}).get("reason", "unknown"))

    # Deploy
    configdrive = build_configdrive(host, data)
    deploy_steps = None
    if has_custom_deploy:
        deploy_steps = [{
            "interface": "deploy",
            "step": custom_deploy["method"],
            "priority": CUSTOM_DEPLOY_PRIORITY,
            "args": {},
        }]

    if has_custom_deploy:
        log_info("provision: requesting deployment", method = custom_deploy["method"])
    else:
        log_info("provision: requesting deployment", image = image_url)
    _, s = change_prov_state(
        prov_id,
        "active",
        configdrive = configdrive,
        deploy_steps = deploy_steps,
    )
    if s == 409:
        log_info("provision: node busy, will retry")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    publish_event("ProvisioningStarted", "Image provisioning started")
    return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

def deprovision(host, restart_on_failure, automated_cleaning_mode):
    """Deprovision (delete instance from) the node."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
    if status == 404:
        return {"dirty": True, "error": "needs-registration"}
    if status != 200 or not node:
        return {"dirty": True, "error": "deprovision: cannot get node"}

    # Sync network_data so the cleaning ramdisk boots with network config.
    nd = resolve_ironic_network_data()
    if nd and nd != node.get("network_data"):
        http_patch("/v1/nodes/" + prov_id, [patch_op("add", "/network_data", nd)])

    state = node.get("provision_state", "")

    # Sync automated_clean setting
    if state in [ACTIVE, DEPLOY_FAIL, DEPLOY_WAIT, ERROR]:
        want_clean = automated_cleaning_mode != "disabled"
        current_clean = node.get("automated_clean")
        if current_clean != None and current_clean != want_clean:
            http_patch("/v1/nodes/" + prov_id, [
                patch_op("replace", "/automated_clean", want_clean),
            ])

    # Request deletion
    if state in [ACTIVE, DEPLOY_FAIL, DEPLOY_WAIT, ERROR]:
        log_info("deprovision: requesting deletion", state = state)
        change_prov_state(prov_id, "deleted")
        publish_event("DeprovisioningStarted", "Image deprovisioning started")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    # Wait or recover
    if state == AVAILABLE:
        publish_event("DeprovisioningComplete", "Image deprovisioning completed")
        return {}

    if state in [DELETING, CLEANING, CLEAN_WAIT]:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    if state == CLEAN_FAIL:
        if restart_on_failure:
            log_info("deprovision: recovering from clean failure")
            if node.get("maintenance", False):
                unset_maintenance(prov_id)
            change_prov_state(prov_id, "manage")
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
        return {"dirty": True, "error": "deprovision: cleaning failed: " + node.get("last_error", "")}

    if state == MANAGEABLE:
        change_prov_state(prov_id, "provide")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    return {"dirty": True, "error": "deprovision: unexpected state " + state}

def delete(host):
    """Delete the node from Ironic entirely."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    if not prov_id:
        return {}

    node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
    if status == 404:
        return {}
    if status != 200 or not node:
        return {"dirty": True, "error": "delete: cannot get node"}

    # Sync network_data so any cleaning triggered by delete has network config.
    nd = resolve_ironic_network_data()
    if nd and nd != node.get("network_data"):
        http_patch("/v1/nodes/" + prov_id, [patch_op("add", "/network_data", nd)])

    state = node.get("provision_state", "")

    if state == VERIFYING:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    # Clean up stale instance_uuid
    if state in [AVAILABLE, MANAGEABLE] and node.get("instance_uuid"):
        http_patch("/v1/nodes/" + prov_id, [
            patch_op("remove", "/instance_uuid"),
        ])

    # Delete directly from safe states
    if state in [AVAILABLE, MANAGEABLE, ENROLL]:
        del_status = http_delete("/v1/nodes/" + prov_id)
        if del_status >= 200 and del_status < 300:
            log_info("delete: node deleted")
            return {}
        if del_status == 409:
            return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
        return {"dirty": True, "error": "delete: DELETE failed with status " + str(del_status)}

    # Force delete via maintenance for other states
    if not node.get("maintenance", False):
        set_maintenance(prov_id, "Preparing to delete")

    del_status = http_delete("/v1/nodes/" + prov_id)
    if del_status >= 200 and del_status < 300:
        log_info("delete: node deleted (was in maintenance)")
        return {}
    if del_status == 409:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
    return {"dirty": True, "error": "delete: DELETE failed with status " + str(del_status)}

def detach(host):
    """Detach node from provisioning system (same as delete)."""
    require_redfish_virtualmedia(host)
    return delete(host)

def power_on(host, _force):
    """Powers on the node."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id)
    if status != 200 or not node:
        return {"dirty": True, "error": "power_on: cannot get node"}

    ps = node.get("power_state", "")
    tps = node.get("target_power_state", "")

    if ps == POWER_ON:
        return {}

    if tps == POWER_ON:
        log_debug("power_on: already transitioning to power on")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    log_info("power_on: powering on")
    _, s = change_power_state(prov_id, POWER_ON)
    if s == 409:
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}
    publish_event("PowerOn", "Host powered on")
    return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

def power_off(host, reboot_mode, force, _automated_cleaning_mode):
    """Powers off the node."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id, ignore_statuses = [404])
    if status == 404:
        return {"dirty": True, "error": "needs-registration"}
    if status != 200 or not node:
        return {"dirty": True, "error": "power_off: cannot get node"}

    state = node.get("provision_state", "")
    ps = node.get("power_state", "")
    tps = node.get("target_power_state", "")

    # Abort in-progress operations
    if state in [INSPECT_WAIT, CLEAN_WAIT]:
        log_info("power_off: aborting " + state + " before power off")
        change_prov_state(prov_id, "abort")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    # Already off or transitioning
    if ps == POWER_OFF:
        return {}

    if tps in [POWER_OFF, SOFT_POWER_OFF]:
        log_debug("power_off: already transitioning to power off")
        return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

    # Power off (soft with fallback to hard)
    if reboot_mode == "soft" and not force:
        log_info("power_off: soft power off")
        _, s = change_power_state(prov_id, SOFT_POWER_OFF, SOFT_POWER_OFF_TIMEOUT)
        if s == 400:
            # Soft power off not supported, fall back to hard
            log_info("power_off: soft power off not supported, using hard power off")
            change_power_state(prov_id, POWER_OFF)
    else:
        log_info("power_off: hard power off")
        change_power_state(prov_id, POWER_OFF)

    publish_event("PowerOff", "Host powered off")
    return {"dirty": True, "requeue_after_seconds": REQUEUE_DELAY}

def get_firmware_settings(host, _include_schema):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return None

def add_bmc_event_subscription(host, _subscription_dict):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return {}

def remove_bmc_event_subscription(host, _subscription_dict):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return {}

def get_firmware_components(host):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return None

def get_data_image_status(host):
    """Not implemented."""
    require_redfish_virtualmedia(host)
    return {"attached": False}

def attach_data_image(host, _url):
    """Not implemented."""
    require_redfish_virtualmedia(host)

def detach_data_image(host):
    """Not implemented."""
    require_redfish_virtualmedia(host)

def has_power_failure(host):
    """Check if node has a power failure fault."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get("/v1/nodes/" + prov_id)
    if status != 200 or not node:
        return False
    return node.get("fault", "") == "power failure"

def get_health(host):
    """Get node health status."""
    require_redfish_virtualmedia(host)
    prov_id = host[HOST_PROVISIONER_ID]
    node, status = http_get(
        "/v1/nodes/" + prov_id,
        API_VERSION_HEALTH,
    )
    if status != 200 or not node:
        return ""
    return node.get("health", "")
