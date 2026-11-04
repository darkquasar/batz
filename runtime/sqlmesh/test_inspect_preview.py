"""Fast checks for reviewed expectation validation; no native pipeline is run."""
import contextlib
from copy import deepcopy
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest


root = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("batz_preview", Path(__file__).with_name("inspect_preview.py"))
preview = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preview)


class ReviewedExpectations(unittest.TestCase):
    def setUp(self):
        self.expected = json.loads((root / "pipelines/aws-cloudtrail/fixtures/api-read.expected.json").read_text())
        self.record = {
            "event": {"action": "object-read"},
            "user": {"id": "arn:aws:iam::111122223333:user/example-responder"},
            "@timestamp": "2026-10-25T12:00:00",
            "source": {"ip": "192.0.2.10"},
            "batz": {"time": {"original": "2026-10-25T12:00:00Z", "fraction_digits": 0}},
        }
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.path = Path(self.scratch.name) / "expected.json"

    def load(self, expected):
        self.path.write_text(json.dumps(expected))
        return preview.reviewed_expectations(self.path, {"api_activity"})

    def assert_integrity_failure(self, function, *args):
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as caught:
            function(*args)
        self.assertEqual(caught.exception.code, 4)

    def test_valid_values(self):
        preview.check_reviewed_values(self.load(self.expected), [self.record], 1)

    def test_wrong_dataset(self):
        self.expected["dataset"] = "unknown-output"
        self.assert_integrity_failure(self.load, self.expected)

    def test_unsupported_version(self):
        self.expected["apiVersion"] = "batz/v9"
        self.assert_integrity_failure(self.load, self.expected)

    def test_malformed_document(self):
        self.path.write_text("[")
        self.assert_integrity_failure(preview.reviewed_expectations, self.path, {"api_activity"})

    def test_boolean_row_count(self):
        self.expected["rows"] = True
        self.assert_integrity_failure(self.load, self.expected)

    def test_missing_expected_field(self):
        self.expected["values"] = {"event.missing": "object-read"}
        self.assert_integrity_failure(preview.check_reviewed_values,
                                      self.load(self.expected), [self.record], 1)

    def test_wrong_value(self):
        wrong = deepcopy(self.record)
        wrong["event"]["action"] = "object-write"
        self.assert_integrity_failure(preview.check_reviewed_values,
                                      self.load(self.expected), [wrong], 1)

    def test_wrong_row_count(self):
        self.assert_integrity_failure(preview.check_reviewed_values,
                                      self.load(self.expected), [], 0)


if __name__ == "__main__":
    unittest.main(verbosity=2)
