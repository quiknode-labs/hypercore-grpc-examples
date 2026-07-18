import importlib.util
from pathlib import Path
import sys
import unittest


MODULE_PATH = Path(__file__).with_name("orderbook_stream_example.py")
sys.path.insert(0, str(MODULE_PATH.parent))
SPEC = importlib.util.spec_from_file_location("orderbook_stream_example", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class L4SnapshotResetTests(unittest.TestCase):
    def test_first_snapshot_is_initial(self):
        self.assertEqual(MODULE.l4_snapshot_reset_kind(1), "initial")

    def test_later_snapshots_are_authoritative_replacements(self):
        self.assertEqual(MODULE.l4_snapshot_reset_kind(2), "replacement")
        self.assertEqual(MODULE.l4_snapshot_reset_kind(10), "replacement")

    def test_invalid_snapshot_count_is_rejected(self):
        for value in (0, -1, True, 1.5):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    MODULE.l4_snapshot_reset_kind(value)


if __name__ == "__main__":
    unittest.main()
