package main

import (
	"reflect"
	"testing"
)

func fixture(objectRoot bool) any {
	tx := map[string]any{
		"tx_hash": "0xraw",
		"signed_actions": []any{
			map[string]any{"action": map[string]any{"type": "order", "orders": []any{map[string]any{"a": 0}}}},
			map[string]any{"action": map[string]any{"type": "cancel", "cancels": []any{map[string]any{"a": "5"}}}},
			map[string]any{"action": map[string]any{"type": "cancelByCloid", "cancels": []any{map[string]any{"asset": 0}}}},
			map[string]any{"action": map[string]any{"type": "batchModify", "modifies": []any{map[string]any{"order": map[string]any{"a": "0"}}}}},
			map[string]any{"action": map[string]any{"type": "modify", "order": map[string]any{"asset": 0}}},
			map[string]any{"action": map[string]any{"type": "twapOrder", "twap": map[string]any{"a": 0}}},
			map[string]any{"action": map[string]any{"type": "twapCancel", "asset": 0}},
			map[string]any{"action": map[string]any{"type": "noop"}},
		},
	}
	if objectRoot {
		return tx
	}
	return []any{"2026-07-17T00:00:00Z", tx}
}

func TestAllOrderTouchingActionsAndRawTuplePreserved(t *testing.T) {
	raw := fixture(false)
	before := fixture(false)
	actions := orderTouchingActions(raw)
	gotTypes := make([]string, 0, len(actions))
	for _, action := range actions {
		gotTypes = append(gotTypes, action.Type)
	}
	wantTypes := []string{"order", "cancel", "cancelByCloid", "batchModify", "modify", "twapOrder", "twapCancel"}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("types = %v, want %v", gotTypes, wantTypes)
	}
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("extractor changed the raw tuple")
	}
	ids := unique(orderTouchingAssetIDs(raw))
	if !reflect.DeepEqual(ids, []string{"0", "5"}) {
		t.Fatalf("asset ids = %v", ids)
	}
}

func TestObjectRootSupported(t *testing.T) {
	if ids := orderTouchingAssetIDs(fixture(true)); len(ids) == 0 || ids[0] != "0" {
		t.Fatalf("object-root ids = %v", ids)
	}
}

func TestInvalidAndNonOrderAssetsIgnored(t *testing.T) {
	raw := map[string]any{"signed_actions": []any{
		map[string]any{"action": map[string]any{"type": "order", "orders": []any{map[string]any{"a": -1}, map[string]any{"a": "BTC"}}}},
		map[string]any{"action": map[string]any{"type": "noop", "a": 0}},
	}}
	if actions := orderTouchingActions(raw); len(actions) != 0 {
		t.Fatalf("unexpected actions: %v", actions)
	}
}
