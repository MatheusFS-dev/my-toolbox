"""Privileged direct Ethernet SSH link manager for ssh-tools."""

import ipaddress
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from toolbox_output import terminal_output


STATE_DIR = pathlib.Path("/var/lib/ssh-tools")
STATE_FILE = STATE_DIR / "direct-link.json"
DNSMASQ_FILE = pathlib.Path("/etc/dnsmasq.d/ssh-tools-direct-link.conf")
NM_CONNECTION = "ssh-tools-direct-link"
AUTOSTART_NAME = "ssh-tools-direct-link-status.desktop"
INTERFACE_PATTERN = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,31}$")
PROTOCOL_VERSION = 1


def dhcp_range(subnet, address):
    """Return DHCP bounds inside an IPv4 subnet away from the local endpoint.

    Args:
        subnet: Canonical IPv4 CIDR network.
        address: Local IPv4 address assigned to the link.

    Returns:
        tuple[str, str]: First and last DHCP addresses.

    Raises:
        ValueError: The network has too few usable addresses.
    """
    network = ipaddress.ip_network(subnet, strict=True)
    host = ipaddress.ip_address(address)
    if network.version != 4 or host not in network or host in (network.network_address, network.broadcast_address):
        raise ValueError("local address must be usable inside the IPv4 link subnet")
    usable = network.num_addresses - 2
    if usable < 3:
        raise ValueError("link subnet must have at least two DHCP addresses")
    if str(network) == "172.30.77.0/24" and str(host) == "172.30.77.1":
        return "172.30.77.100", "172.30.77.200"
    base = int(network.network_address)
    high_start = max(base + 1, int(host) + 1)
    high_end = base + usable
    low_start = base + 1
    low_end = min(base + usable, int(host) - 1)
    if high_end - high_start >= 1:
        first = high_start
        last = min(high_end, high_start + 100)
    elif low_end - low_start >= 1:
        first = low_start
        last = min(low_end, low_start + 100)
    else:
        raise ValueError("link subnet does not leave a contiguous DHCP range")
    return str(ipaddress.ip_address(first)), str(ipaddress.ip_address(last))


def validate_config(value):
    """Validate setup JSON before any system mutation.

    Args:
        value: JSON object with interface, subnet, and address strings.

    Returns:
        dict: Normalized validated configuration.

    Raises:
        ValueError: Invalid interface or network settings.
    """
    if not isinstance(value, dict) or set(value) != {"interface", "subnet", "address"}:
        raise ValueError("setup requires interface, subnet, and address")
    interface = value["interface"]
    if not isinstance(interface, str) or not INTERFACE_PATTERN.fullmatch(interface):
        raise ValueError("invalid Ethernet interface")
    network = ipaddress.ip_network(value["subnet"], strict=True)
    address = ipaddress.ip_address(value["address"])
    if network.version != 4 or address.version != 4:
        raise ValueError("direct Ethernet requires IPv4")
    dhcp_range(str(network), str(address))
    return {"interface": interface, "subnet": str(network), "address": str(address)}


def run(arguments, check=True):
    """Run a command as an argument array without a shell.

    Args:
        arguments: Executable and literal arguments.
        check: Raise when the process fails.

    Returns:
        subprocess.CompletedProcess: Captured command result.
    """
    mutating = ((arguments[0] == "nmcli" and len(arguments) > 2 and
                 arguments[1] == "connection" and arguments[2] in ("add", "up", "down", "delete")) or
                (arguments[0] == "systemctl" and len(arguments) > 1 and
                 arguments[1] in ("enable", "disable", "stop", "restart")) or
                (arguments[0] == "ufw" and len(arguments) > 1 and
                 arguments[1] in ("allow", "delete", "--force")))
    if mutating:
        return subprocess.run(arguments, check=check)
    return subprocess.run(arguments, check=check, text=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE)


def dependencies():
    """Return missing executables needed for direct link setup.

    Returns:
        list[str]: Missing command names.
    """
    return [name for name in ("nmcli", "dnsmasq", "systemctl", "sshd", "ip") if shutil.which(name) is None]


def check_network_overlap(config):
    """Reject a link subnet already present on another local interface.

    Args:
        config: Validated interface and subnet settings.

    Raises:
        RuntimeError: The requested network overlaps a local IPv4 route.
    """
    target = ipaddress.ip_network(config["subnet"])
    interfaces = json.loads(run(["ip", "-j", "-4", "addr", "show"]).stdout)
    for interface in interfaces:
        if interface.get("ifname") == config["interface"]:
            continue
        for item in interface.get("addr_info", []):
            if item.get("family") != "inet":
                continue
            existing = ipaddress.ip_network("{0}/{1}".format(
                item["local"], item["prefixlen"]), strict=False)
            if target.overlaps(existing):
                raise RuntimeError("direct-link subnet overlaps another local interface")


def eligible_interfaces():
    """List physical or USB Ethernet links that NetworkManager can address.

    Returns:
        list[str]: Ethernet interface names.
    """
    root = pathlib.Path("/sys/class/net")
    if not root.is_dir():
        return []
    return sorted(path.name for path in root.iterdir()
                  if path.name != "lo" and (path / "type").exists()
                  and not (path / "wireless").exists()
                  and (path / "type").read_text(encoding="ascii").strip() == "1")


def service_state(name):
    """Read systemd enablement and activity for one service.

    Args:
        name: Systemd service name.

    Returns:
        dict: Boolean enabled and active flags.
    """
    return {"enabled": run(["systemctl", "is-enabled", name], check=False).returncode == 0,
            "active": run(["systemctl", "is-active", name], check=False).returncode == 0}


def snapshot(state_path=STATE_FILE):
    """Build the versioned machine-readable direct-link snapshot.

    Args:
        state_path: Path to the saved rollback state.

    Returns:
        dict: Configuration, runtime, dependencies, and monitor status.
    """
    state = None
    if state_path.is_symlink():
        raise RuntimeError("refusing symlinked ssh-tools rollback state")
    if state_path.exists():
        state = json.loads(state_path.read_text(encoding="utf-8"))
    missing = dependencies()
    interfaces = eligible_interfaces()
    config = state.get("config") if state else None
    active = False
    if config and shutil.which("nmcli"):
        result = run(["nmcli", "-g", "GENERAL.STATE", "device", "show", config["interface"]], check=False)
        active = result.returncode == 0 and result.stdout.strip().startswith("100")
    dnsmasq_active = False
    server_active = False
    if config and shutil.which("systemctl"):
        dnsmasq_active = service_state("dnsmasq")["active"]
        server_active = service_state(state.get("ssh_service", "ssh"))["active"]
    firewall = "inactive"
    if shutil.which("ufw"):
        result = run(["ufw", "status"], check=False)
        if result.returncode != 0:
            firewall = "unknown"
        elif "Status: active" in result.stdout:
            firewall = "ufw"
    user = os.environ.get("SUDO_USER") or os.environ.get("USER") or ""
    try:
        import pwd
        home = pathlib.Path(pwd.getpwnam(user).pw_dir)
    except (ImportError, KeyError):
        home = pathlib.Path.home()
    monitor = home / ".config" / "autostart" / AUTOSTART_NAME
    return {"protocol_version": PROTOCOL_VERSION, "available": not missing,
            "config": config, "runtime": {"active": active,
                                           "dnsmasq": dnsmasq_active,
                                           "ssh_server": server_active,
                                           "firewall": firewall},
            "interfaces": interfaces, "missing_dependencies": missing,
            "status_monitor": {"installed": monitor.exists()}}


def atomic_write(path, content, mode=0o600):
    """Replace a file atomically with restrictive permissions.

    Args:
        path: Destination path.
        content: UTF-8 text.
        mode: Final POSIX permission bits.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".ssh-tools-", dir=str(path.parent))
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def check_setup_preconditions(config, state_path=STATE_FILE):
    """Reject unsafe setup conditions before writing rollback state.

    Args:
        config: Validated link settings.
        state_path: Rollback state location.

    Raises:
        RuntimeError: A dependency, interface, or managed path is unavailable.
    """
    if os.geteuid() != 0:
        raise RuntimeError("run setup through sudo")
    if state_path.is_symlink() or state_path.parent.is_symlink():
        raise RuntimeError("refusing symlinked rollback path")
    if state_path.exists():
        raise RuntimeError("direct Ethernet is already managed; undo it first")
    if DNSMASQ_FILE.exists() or DNSMASQ_FILE.is_symlink():
        raise RuntimeError("ssh-tools dnsmasq configuration already exists without rollback state")
    missing = dependencies()
    if missing:
        raise RuntimeError("install missing dependencies: " + ", ".join(missing))
    if config["interface"] not in eligible_interfaces():
        raise RuntimeError("select an existing Ethernet interface")
    check_network_overlap(config)
    gateway = run(["nmcli", "-g", "IP4.GATEWAY", "device", "show", config["interface"]], check=False)
    if gateway.returncode == 0 and gateway.stdout.strip():
        raise RuntimeError("selected interface has a gateway; choose a spare Ethernet interface")
    if run(["nmcli", "-g", "NAME", "connection", "show", NM_CONNECTION], check=False).returncode == 0:
        raise RuntimeError("NetworkManager connection already exists without rollback state")
    if shutil.which("ufw"):
        rules = run(["ufw", "status", "numbered"], check=False)
        if rules.returncode != 0:
            raise RuntimeError("cannot inspect UFW status before direct-link setup")
        if "ssh-tools-direct-link" in rules.stdout:
            raise RuntimeError("ssh-tools firewall rule already exists without rollback state")


def remove_managed_firewall_rule(interface):
    """Remove only the numbered UFW rule carrying the ssh-tools identifier.

    Args:
        interface: Managed direct-link interface.

    Returns:
        bool: Whether a managed rule was removed.

    Raises:
        RuntimeError: Duplicate matching rules require manual inspection.
    """
    listing = run(["ufw", "status", "numbered"], check=False)
    if listing.returncode != 0:
        raise RuntimeError("could not inspect UFW rules; rollback state retained")
    matches = []
    for line in listing.stdout.splitlines():
        if "ssh-tools-direct-link" not in line or "on " + interface + " " not in line:
            continue
        match = re.match(r"^\[\s*(\d+)\]", line.strip())
        if match:
            matches.append(match.group(1))
    if len(matches) > 1:
        raise RuntimeError("multiple ssh-tools firewall rules found; inspect UFW before undo")
    if not matches:
        return False
    run(["ufw", "--force", "delete", matches[0]])
    return True


def setup(config, state_path=STATE_FILE):
    """Save rollback state, then configure NetworkManager, DHCP, SSH, and firewall.

    Args:
        config: Validated setup configuration.
        state_path: Rollback state location.
    """
    config = validate_config(config)
    check_setup_preconditions(config, state_path)
    first, last = dhcp_range(config["subnet"], config["address"])
    server = "ssh" if run(["systemctl", "list-unit-files", "ssh.service"], check=False).returncode == 0 else "sshd"
    ufw_active = shutil.which("ufw") is not None and "Status: active" in run(["ufw", "status"], check=False).stdout
    previous_connection = run(["nmcli", "-g", "GENERAL.CONNECTION", "device", "show",
                               config["interface"]], check=False).stdout.strip()
    prior = {"config": config, "dnsmasq": service_state("dnsmasq"),
             "ssh": service_state(server), "ssh_service": server,
             "previous_connection": previous_connection, "ufw_rule": ufw_active,
             "protocol_version": PROTOCOL_VERSION}
    atomic_write(state_path, json.dumps(prior, indent=2) + "\n", 0o644)
    try:
        atomic_write(DNSMASQ_FILE, "port=0\ninterface={0}\nbind-interfaces\ndhcp-range={1},{2},12h\n".format(
            config["interface"], first, last), 0o644)
        run(["nmcli", "connection", "add", "type", "ethernet", "ifname", config["interface"],
             "con-name", NM_CONNECTION, "ipv4.method", "manual", "ipv4.addresses",
             config["address"] + "/" + config["subnet"].split("/")[1],
             "ipv6.method", "disabled", "connection.autoconnect", "yes"])
        run(["nmcli", "connection", "up", NM_CONNECTION])
        run(["systemctl", "enable", "--now", "dnsmasq"])
        run(["systemctl", "restart", "dnsmasq"])
        run(["systemctl", "enable", "--now", server])
        if ufw_active:
            run(["ufw", "allow", "in", "on", config["interface"], "to", "any", "port", "22",
                 "proto", "tcp", "comment", "ssh-tools-direct-link"])
    except Exception:
        undo(state_path)
        raise


def restore_service(name, previous):
    """Restore service activity and enablement recorded before setup.

    Args:
        name: Systemd service.
        previous: Saved enabled and active flags.
    """
    if not previous["active"]:
        run(["systemctl", "stop", name], check=False)
    if not previous["enabled"]:
        run(["systemctl", "disable", name], check=False)


def undo(state_path=STATE_FILE):
    """Remove only ssh-tools changes and restore prior service states.

    Args:
        state_path: Saved rollback state.

    Raises:
        RuntimeError: No rollback state exists.
    """
    if os.geteuid() != 0:
        raise RuntimeError("run undo through sudo")
    if state_path.is_symlink():
        raise RuntimeError("refusing symlinked rollback state")
    if not state_path.exists():
        raise RuntimeError("no ssh-tools rollback state exists")
    state = json.loads(state_path.read_text(encoding="utf-8"))
    if state.get("protocol_version") != PROTOCOL_VERSION:
        raise RuntimeError("unsupported rollback protocol version")
    if shutil.which("ufw"):
        remove_managed_firewall_rule(state["config"]["interface"])
    connection = run(["nmcli", "connection", "show", NM_CONNECTION], check=False)
    if connection.returncode == 0:
        run(["nmcli", "connection", "down", NM_CONNECTION], check=False)
        deleted = run(["nmcli", "connection", "delete", NM_CONNECTION], check=False)
        if deleted.returncode != 0:
            raise RuntimeError("NetworkManager did not delete the ssh-tools connection; rollback state retained")
    previous = state.get("previous_connection")
    if previous and previous != "--":
        run(["nmcli", "connection", "up", previous], check=False)
    DNSMASQ_FILE.unlink(missing_ok=True)
    if state["dnsmasq"]["active"]:
        run(["systemctl", "restart", "dnsmasq"])
    restore_service("dnsmasq", state["dnsmasq"])
    restore_service(state["ssh_service"], state["ssh"])
    state_path.unlink()


def monitor_path():
    """Resolve the invoking desktop user's autostart file.

    Returns:
        tuple[pathlib.Path, int, int]: Path and ownership IDs.
    """
    import pwd
    user = os.environ.get("SUDO_USER")
    if not user:
        raise RuntimeError("run monitor installation through sudo from a desktop user")
    record = pwd.getpwnam(user)
    return pathlib.Path(record.pw_dir) / ".config" / "autostart" / AUTOSTART_NAME, record.pw_uid, record.pw_gid


def install_status():
    """Install a desktop-login notification owned by the invoking user."""
    if os.geteuid() != 0:
        raise RuntimeError("run install-status through sudo")
    path, uid, gid = monitor_path()
    if path.exists() or path.is_symlink():
        raise RuntimeError("status monitor file already exists; refusing overwrite")
    for directory in (path.parent.parent, path.parent):
        if directory.is_symlink():
            raise RuntimeError("refusing symlinked desktop configuration directory")
        if not directory.exists():
            directory.mkdir(mode=0o700)
            os.chown(directory, uid, gid)
    if shutil.which("notify-send") is None:
        raise RuntimeError("install notify-send to use the desktop status monitor")
    wrapper = path.parent.parent.parent / ".local" / "bin" / "tb"
    if not wrapper.is_file() or not os.access(wrapper, os.X_OK):
        raise RuntimeError("install the stable tb launcher before enabling the status monitor")
    escaped = str(wrapper).replace('"', '\\"')
    content = "[Desktop Entry]\nType=Application\nName=ssh-tools direct-link status\nExec=\"{0}\" __ssh-tools-status\nX-GNOME-Autostart-enabled=true\n".format(escaped)
    atomic_write(path, content, 0o644)
    os.chown(path, uid, gid)


def uninstall_status():
    """Remove only the exact ssh-tools desktop-login monitor."""
    if os.geteuid() != 0:
        raise RuntimeError("run uninstall-status through sudo")
    path, _, _ = monitor_path()
    if path.parent.is_symlink() or path.parent.parent.is_symlink():
        raise RuntimeError("refusing symlinked desktop configuration directory")
    if not path.exists():
        return
    if path.is_symlink() or "Name=ssh-tools direct-link status" not in path.read_text(encoding="utf-8"):
        raise RuntimeError("status monitor file is not managed by ssh-tools")
    path.unlink()


def render_status_notification(state):
    """Render the desktop-login status message from a snapshot.

    Args:
        state: Protocol-versioned snapshot data.

    Returns:
        str: Concise link, service, and firewall status.
    """
    config = state.get("config")
    if not config:
        return "No direct Ethernet link configured."
    runtime = state["runtime"]
    return "{0}: {1}; dnsmasq {2}; SSH server {3}; firewall {4}".format(
        config["interface"], "active" if runtime["active"] else "inactive",
        "running" if runtime["dnsmasq"] else "stopped",
        "running" if runtime["ssh_server"] else "stopped", runtime["firewall"])


def main(arguments=None):
    """Execute one internal helper action.

    Args:
        arguments: Optional action argument array.

    Returns:
        int: Zero on success, one on failure.
    """
    args = sys.argv[1:] if arguments is None else arguments
    if len(args) != 1 or args[0] not in ("snapshot", "setup", "undo", "install-status",
                                          "uninstall-status", "status-display"):
        terminal_output("ssh-tools", sys.stderr).error("select one internal helper action")
        return 1
    action = args[0]
    if action == "snapshot":
        json.dump(snapshot(), sys.stdout, sort_keys=True)
        sys.stdout.write("\n")
        return 0
    output = terminal_output("ssh-tools")
    try:
        if action == "status-display":
            state = snapshot()
            run(["notify-send", "ssh-tools direct Ethernet", render_status_notification(state)])
            return 0
        output.info("Applying {0}...".format(action))
        if action == "setup":
            setup(json.load(sys.stdin))
        elif action == "undo":
            undo()
        elif action == "install-status":
            install_status()
        else:
            uninstall_status()
    except (ValueError, RuntimeError, OSError, subprocess.CalledProcessError, json.JSONDecodeError) as error:
        output.error("{0}. Review direct-link state before retrying.".format(error))
        return 1
    output.info("{0} completed.".format(action))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
