"""Direct Ethernet helper contract tests."""

import importlib.util
import pathlib
import unittest
from unittest import mock
import tempfile


SOURCE = pathlib.Path(__file__).resolve().parents[1] / "direct_link.py"


def load_helper():
    """Load the helper without executing its CLI."""
    specification = importlib.util.spec_from_file_location("ssh_tools_direct_link", SOURCE)
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    return module


class DirectLinkTests(unittest.TestCase):
    """Validate safe network inputs and machine-readable snapshots."""

    def test_default_dhcp_range(self):
        """Reserve the host address and serve a bounded DHCP range."""
        helper = load_helper()
        self.assertEqual(helper.dhcp_range("172.30.77.0/24", "172.30.77.1"),
                         ("172.30.77.100", "172.30.77.200"))

    def test_rejects_invalid_interface_and_network(self):
        """Reject command injection and host addresses outside the link."""
        helper = load_helper()
        for interface in ("", "eth0;touch /tmp/x", "../eth0"):
            with self.assertRaises(ValueError):
                helper.validate_config({"interface": interface,
                                        "subnet": "172.30.77.0/24",
                                        "address": "172.30.77.1"})
        with self.assertRaises(ValueError):
            helper.validate_config({"interface": "eth0", "subnet": "172.30.77.0/24",
                                    "address": "192.0.2.1"})

    def test_rejects_overlapping_local_network(self):
        """Do not put a direct link on a subnet already routed elsewhere."""
        helper = load_helper()
        addresses = [{"ifname": "wlan0", "addr_info": [
            {"family": "inet", "local": "172.30.77.9", "prefixlen": 24}]}]
        with mock.patch.object(helper, "run", return_value=mock.Mock(stdout=__import__("json").dumps(addresses))):
            with self.assertRaisesRegex(RuntimeError, "overlaps"):
                helper.check_network_overlap({"interface": "eth0", "subnet": "172.30.77.0/24"})

    def test_snapshot_has_protocol_and_dependencies(self):
        """Expose a protocol version and missing dependencies in JSON state."""
        helper = load_helper()
        snapshot = helper.snapshot(state_path=pathlib.Path("/nonexistent/ssh-tools-state.json"))
        self.assertEqual(snapshot["protocol_version"], 1)
        self.assertIn("missing_dependencies", snapshot)
        self.assertIn("interfaces", snapshot)
        self.assertIn("dnsmasq", snapshot["runtime"])
        self.assertIn("ssh_server", snapshot["runtime"])
        self.assertIn("firewall", snapshot["runtime"])

    def test_status_monitor_renders_service_state(self):
        """Desktop notification includes the link and critical services."""
        helper = load_helper()
        message = helper.render_status_notification({"config": {"interface": "eth0"},
            "runtime": {"active": True, "dnsmasq": True, "ssh_server": False, "firewall": "ufw"}})
        for expected in ("eth0", "active", "dnsmasq running", "SSH server stopped", "ufw"):
            self.assertIn(expected, message)

    def test_setup_saves_rollback_before_system_mutation(self):
        """Persist state before creating managed network files or running commands."""
        helper = load_helper()
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            state = root / "state.json"
            dnsmasq = root / "dnsmasq.conf"
            events = []

            def command(arguments, check=True):
                events.append((list(arguments), state.exists()))
                return mock.Mock(returncode=0, stdout="")

            with mock.patch.object(helper, "DNSMASQ_FILE", dnsmasq), \
                    mock.patch.object(helper, "check_setup_preconditions"), \
                    mock.patch.object(helper, "service_state", return_value={"enabled": False, "active": False}), \
                    mock.patch.object(helper, "run", side_effect=command), \
                    mock.patch.object(helper.shutil, "which", return_value=None):
                helper.setup({"interface": "eth0", "subnet": "172.30.77.0/24",
                              "address": "172.30.77.1"}, state)
            self.assertTrue(state.exists())
            mutations = [saved for arguments, saved in events
                         if (arguments[0] == "nmcli" and arguments[1] == "connection") or
                         (arguments[0] == "ufw" and arguments[1] == "allow") or
                         (arguments[0] == "systemctl" and arguments[1] == "enable")]
            self.assertTrue(mutations)
            self.assertTrue(all(mutations))
            self.assertIn("interface=eth0", dnsmasq.read_text(encoding="utf-8"))

    def test_undo_removes_only_managed_files(self):
        """Restore saved service state and retain unrelated files."""
        helper = load_helper()
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            state = root / "state.json"
            dnsmasq = root / "dnsmasq.conf"
            unrelated = root / "other.conf"
            dnsmasq.write_text("managed", encoding="utf-8")
            unrelated.write_text("keep", encoding="utf-8")
            state.write_text('{"protocol_version":1,"config":{"interface":"eth0"},'
                             '"ufw_rule":false,"dnsmasq":{"enabled":false,"active":false},'
                             '"ssh":{"enabled":true,"active":true},"ssh_service":"ssh"}', encoding="utf-8")
            with mock.patch.object(helper, "DNSMASQ_FILE", dnsmasq), \
                    mock.patch.object(helper.os, "geteuid", return_value=0), \
                    mock.patch.object(helper, "run", return_value=mock.Mock(returncode=0, stdout="")):
                helper.undo(state)
            self.assertFalse(state.exists())
            self.assertFalse(dnsmasq.exists())
            self.assertTrue(unrelated.exists())

    def test_failed_undo_keeps_rollback_state(self):
        """Keep recovery data when NetworkManager refuses to delete the link."""
        helper = load_helper()
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            state = root / "state.json"
            state.write_text('{"protocol_version":1,"config":{"interface":"eth0"},'
                             '"ufw_rule":false,"dnsmasq":{"enabled":false,"active":false},'
                             '"ssh":{"enabled":true,"active":true},"ssh_service":"ssh"}', encoding="utf-8")

            def command(arguments, check=True):
                if arguments[:3] == ["nmcli", "connection", "delete"]:
                    return mock.Mock(returncode=1, stdout="")
                return mock.Mock(returncode=0, stdout="")

            with mock.patch.object(helper.os, "geteuid", return_value=0), \
                    mock.patch.object(helper, "run", side_effect=command), \
                    mock.patch.object(helper, "DNSMASQ_FILE", root / "dnsmasq.conf"):
                with self.assertRaises(RuntimeError):
                    helper.undo(state)
            self.assertTrue(state.exists())

    def test_firewall_rollback_targets_only_named_rule(self):
        """Delete a numbered ssh-tools UFW rule without touching other SSH rules."""
        helper = load_helper()
        calls = []

        def command(arguments, check=True):
            calls.append(list(arguments))
            return mock.Mock(returncode=0, stdout="[ 1] 22/tcp ALLOW IN Anywhere\n"
                             "[ 2] 22/tcp on eth0 ALLOW IN Anywhere # ssh-tools-direct-link\n")

        with mock.patch.object(helper, "run", side_effect=command):
            helper.remove_managed_firewall_rule("eth0")
        self.assertEqual(calls[-1], ["ufw", "--force", "delete", "2"])


if __name__ == "__main__":
    unittest.main()
