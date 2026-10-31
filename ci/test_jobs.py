"""Bounded orchestration using real small processes and isolated output roots."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

spec = importlib.util.spec_from_file_location('lanes', Path(__file__).with_name('test.py'))
lanes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lanes)


class SelectedWorkers(unittest.TestCase):
    def test_two_independent_jobs_overlap_with_isolated_state(self):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            barrier = threading.Barrier(2)
            lock = threading.Lock()
            active = 0
            peak = 0

            def worker(identity):
                def work():
                    nonlocal active, peak
                    with lock:
                        active += 1
                        peak = max(peak, active)
                    try:
                        barrier.wait(timeout=0.3)
                    except threading.BrokenBarrierError:
                        pass
                    destination = root / identity
                    destination.mkdir()
                    subprocess.run([sys.executable, '-I', '-c', "import pathlib,sys; pathlib.Path('state.json').write_text(sys.argv[1])", identity], cwd=destination, check=True)
                    with lock:
                        active -= 1
                return work

            lanes.run_jobs([(1, worker('a')), (1, worker('b'))], locks_root=root / 'locks')
            self.assertEqual(peak, 2, 'selected independent jobs ran serially')
            self.assertEqual((root / 'a/state.json').read_text(), 'a')
            self.assertEqual((root / 'b/state.json').read_text(), 'b')

    def test_package_parallelism_reserves_both_native_slots(self):
        with tempfile.TemporaryDirectory() as scratch:
            lock = threading.Lock()
            active = 0
            peak = 0

            def worker(slots):
                def work():
                    nonlocal active, peak
                    with lock:
                        active += slots
                        peak = max(peak, active)
                    subprocess.run([sys.executable, '-I', '-c', 'import time; time.sleep(0.05)'], check=True)
                    with lock:
                        active -= slots
                return work

            lanes.run_jobs([(2, worker(2)), (1, worker(1))], locks_root=Path(scratch) / 'locks')
            self.assertLessEqual(peak, 2, 'nested Go and job parallelism exceeded the host native budget')


if __name__ == '__main__':
    unittest.main()
