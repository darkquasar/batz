"""Public wrapper refusal checks; requested work must never silently pass."""
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]


class RequestedLanes(unittest.TestCase):
    def test_unknown_requested_pipeline_is_refused(self):
        result = subprocess.run([str(ROOT / 'ci/test'), 'e2e', 'missing-pipeline'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn('missing-pipeline', result.stderr)


if __name__ == '__main__':
    unittest.main()
