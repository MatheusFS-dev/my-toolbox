import unittest

from monitor_runtime.protocol import ProtocolError, validate_request


class ProtocolTests(unittest.TestCase):
    def test_run_request_requires_exact_supported_protocol(self):
        with self.assertRaisesRegex(ProtocolError, "protocol_version"):
            validate_request({"protocol_version": 2, "type": "run", "scripts": ["a.py"]})

    def test_protocol_rejects_unknown_request_type(self):
        with self.assertRaisesRegex(ProtocolError, "request type"):
            validate_request({"protocol_version": 1, "type": "secret_dump"})

    def test_run_request_never_accepts_credentials_inline(self):
        with self.assertRaisesRegex(ProtocolError, "secret"):
            validate_request({"protocol_version": 1, "type": "run", "scripts": ["a.py"], "password": "x"})

    def test_run_titles_must_match_scripts(self):
        """Reject title lists that cannot map one-to-one to scripts."""
        with self.assertRaisesRegex(ProtocolError, "titles"):
            validate_request({
                "protocol_version": 1,
                "type": "run",
                "scripts": ["a.py", "b.py"],
                "titles": ["Only one"],
                "interpreter": "/usr/bin/python3",
            })

    def test_rejects_invalid_gpu_uuid(self):
        request = {"protocol_version": 1, "type": "run", "scripts": ["a.py"], "interpreter": "python", "gpu_uuid": "invalid"}
        with self.assertRaisesRegex(ProtocolError, "gpu_uuid"):
            validate_request(request)

    def test_script_args_must_match_scripts_and_contain_strings_without_nuls(self):
        request = {"protocol_version": 1, "type": "run", "scripts": ["a.py"], "interpreter": "python", "script_args": [["--epochs", "10"]]}
        self.assertEqual(validate_request(request)["script_args"], [["--epochs", "10"]])
        for invalid in ([], [["ok"], ["extra"]], [["bad\x00value"]], ["--epochs"]):
            request["script_args"] = invalid
            with self.assertRaisesRegex(ProtocolError, "script_args"):
                validate_request(request)
