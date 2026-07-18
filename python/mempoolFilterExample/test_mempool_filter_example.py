import copy
import importlib.util
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("mempool_filter_example.py")
SPEC = importlib.util.spec_from_file_location("mempool_filter_example", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


def fixture(object_root=False):
    tx = {
        "tx_hash": "0xraw",
        "signed_actions": [
            {"action": {"type": "order", "orders": [{"a": 0}]}},
            {"action": {"type": "cancel", "cancels": [{"a": "5"}]}},
            {"action": {"type": "cancelByCloid", "cancels": [{"asset": 0}]}},
            {"action": {"type": "batchModify", "modifies": [{"order": {"a": "0"}}]}},
            {"action": {"type": "modify", "order": {"asset": 0}}},
            {"action": {"type": "twapOrder", "twap": {"a": 0}}},
            {"action": {"type": "twapCancel", "asset": 0}},
            {"action": {"type": "noop"}},
        ],
    }
    return tx if object_root else ["2026-07-17T00:00:00Z", tx]


class MempoolFilterExtractionTests(unittest.TestCase):
    def test_all_order_touching_actions_and_raw_tuple_preserved(self):
        raw = fixture()
        before = copy.deepcopy(raw)
        actions = MODULE.order_touching_actions(raw)
        self.assertEqual(
            [action["type"] for action in actions],
            ["order", "cancel", "cancelByCloid", "batchModify", "modify", "twapOrder", "twapCancel"],
        )
        self.assertEqual(set(MODULE.order_touching_asset_ids(raw)), {"0", "5"})
        self.assertEqual(raw, before)

    def test_object_root_supported(self):
        self.assertIn("0", MODULE.order_touching_asset_ids(fixture(object_root=True)))

    def test_non_order_and_invalid_assets_ignored(self):
        raw = {"signed_actions": [
            {"action": {"type": "order", "orders": [{"a": -1}, {"a": "BTC"}]}},
            {"action": {"type": "noop", "a": 0}},
        ]}
        self.assertEqual(MODULE.order_touching_actions(raw), [])


if __name__ == "__main__":
    unittest.main()
