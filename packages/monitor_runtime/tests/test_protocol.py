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
