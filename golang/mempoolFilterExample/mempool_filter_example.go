package main

// This example filters raw MEMPOOL_TXS with coin=BTC. Raptor resolves the coin
// dynamically to numeric asset IDs and matches all order-touching action types.
// Matching transactions retain the original raw JSON tuple/object.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/example/hyperliquid-grpc/proto"
)

const (
	defaultEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	defaultToken    = "YOUR_QUICKNODE_TOKEN"
)

var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

type touchingAction struct {
	Type     string
	AssetIDs []string
}

func signedActions(value any) []map[string]any {
	tx := value
	if tuple, ok := value.([]any); ok && len(tuple) > 1 {
		tx = tuple[1]
	}
	object, ok := tx.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := object["signed_actions"].([]any)
	if !ok {
		return nil
	}
	actions := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if action, ok := item.(map[string]any); ok {
			actions = append(actions, action)
		}
	}
	return actions
}

func assetID(value any) (string, bool) {
	switch typed := value.(type) {
	case json.Number:
		raw := typed.String()
		if raw != "" && !strings.ContainsAny(raw, ".-+eE") {
			return raw, true
		}
	case string:
		if typed != "" && strings.IndexFunc(typed, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return typed, true
		}
	case int:
		if typed >= 0 {
			return fmt.Sprint(typed), true
		}
	case uint64:
		return fmt.Sprint(typed), true
	}
	return "", false
}

func directAssets(value any) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	assets := make([]string, 0, 2)
	for _, field := range []string{"a", "asset"} {
		if id, ok := assetID(object[field]); ok {
			assets = append(assets, id)
		}
	}
	return assets
}

func arrayItems(value any) []any {
	items, _ := value.([]any)
	return items
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func orderTouchingActions(value any) []touchingAction {
	matches := make([]touchingAction, 0)
	for _, signed := range signedActions(value) {
		action, ok := signed["action"].(map[string]any)
		if !ok {
			continue
		}
		actionType, _ := action["type"].(string)
		assets := make([]string, 0)
		switch actionType {
		case "order":
			for _, item := range arrayItems(action["orders"]) {
				assets = append(assets, directAssets(item)...)
			}
		case "cancel", "cancelByCloid":
			for _, item := range arrayItems(action["cancels"]) {
				assets = append(assets, directAssets(item)...)
			}
		case "batchModify":
			for _, item := range arrayItems(action["modifies"]) {
				if modify, ok := item.(map[string]any); ok {
					assets = append(assets, directAssets(modify["order"])...)
				}
				assets = append(assets, directAssets(item)...)
			}
		case "modify":
			assets = append(assets, directAssets(action["order"])...)
			assets = append(assets, directAssets(action)...)
		case "twapOrder":
			assets = append(assets, directAssets(action["twap"])...)
		case "twapCancel":
			assets = append(assets, directAssets(action)...)
		}
		if assets = unique(assets); len(assets) > 0 {
			matches = append(matches, touchingAction{Type: actionType, AssetIDs: assets})
		}
	}
	return matches
}

func orderTouchingAssetIDs(value any) []string {
	var assets []string
	for _, action := range orderTouchingActions(value) {
		assets = append(assets, action.AssetIDs...)
	}
	return assets
}

func parseJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}

func decodeData(data string) (string, error) {
	raw := []byte(data)
	if len(raw) < 4 || string(raw[:4]) != string(zstdMagic) {
		return data, nil
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return "", err
	}
	defer decoder.Close()
	decoded, err := decoder.DecodeAll(raw, nil)
	return string(decoded), err
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func splitValues(raw string) []string {
	var values []string
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return unique(values)
}

func main() {
	coin := flag.String("coin", "BTC", "Comma-separated coin names (OR semantics)")
	filterField := flag.String("filter-field", "coin", "Virtual filter field: coin or coins")
	expectedRaw := flag.String("expected-asset-ids", "0", "Numeric IDs used only to validate returned raw data")
	maxMessages := flag.Int("max-messages", 5, "Number of messages required for success")
	timeout := flag.Duration("timeout", 60*time.Second, "Bounded subscription duration")
	unfiltered := flag.Bool("unfiltered", false, "Sample raw MEMPOOL_TXS without a filter")
	expectNoMatch := flag.Bool("expect-no-match", false, "Pass only if the filter returns no data")
	printRaw := flag.Bool("print-raw", false, "Print each original raw JSON payload")
	flag.Parse()

	endpoint := env("GRPC_ENDPOINT", defaultEndpoint)
	token := env("AUTH_TOKEN", env("QN_AUTH_TOKEN", defaultToken))
	if endpoint == defaultEndpoint || token == defaultToken {
		fmt.Fprintln(os.Stderr, "FAILED: set GRPC_ENDPOINT and AUTH_TOKEN (or QN_AUTH_TOKEN)")
		os.Exit(2)
	}
	if *maxMessages <= 0 || *timeout <= 0 || (*unfiltered && *expectNoMatch) {
		fmt.Fprintln(os.Stderr, "FAILED: invalid max-messages, timeout, or mode combination")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	target := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	streamCtx := metadata.AppendToOutgoingContext(ctx, "x-token", token)
	stream, err := pb.NewStreamingClient(conn).StreamData(streamCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: stream: %v\n", err)
		os.Exit(1)
	}
	filters := map[string]*pb.FilterValues{}
	if !*unfiltered {
		filters[*filterField] = &pb.FilterValues{Values: splitValues(*coin)}
	}
	err = stream.Send(&pb.SubscribeRequest{Request: &pb.SubscribeRequest_Subscribe{Subscribe: &pb.StreamSubscribe{
		StreamType: pb.StreamType_MEMPOOL_TXS,
		Filters:    filters,
		FilterName: "mempool-coin-filter",
	}}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: subscribe: %v\n", err)
		os.Exit(1)
	}

	expected := make(map[string]struct{})
	for _, id := range splitValues(*expectedRaw) {
		expected[id] = struct{}{}
	}
	received := 0
	for {
		response, recvErr := stream.Recv()
		if recvErr != nil {
			if *expectNoMatch && (errors.Is(recvErr, context.DeadlineExceeded) || status.Code(recvErr).String() == "DeadlineExceeded") {
				fmt.Printf("PASS: no MEMPOOL_TXS messages matched within %s\n", timeout.String())
				return
			}
			if recvErr == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "FAILED: receive: %v\n", recvErr)
			os.Exit(1)
		}
		data := response.GetData()
		if data == nil {
			continue
		}
		if *expectNoMatch {
			fmt.Fprintln(os.Stderr, "FAILED: deliberately non-matching coin returned a transaction")
			os.Exit(1)
		}
		raw, err := decodeData(data.Data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: decode: %v\n", err)
			os.Exit(1)
		}
		value, err := parseJSON(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: JSON: %v\n", err)
			os.Exit(1)
		}
		matches := make([]string, 0)
		for _, id := range orderTouchingAssetIDs(value) {
			if _, ok := expected[id]; ok {
				matches = append(matches, id)
			}
		}
		matches = unique(matches)
		if !*unfiltered && len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "FAILED: raw transaction lacks expected asset; observed=%v\n", orderTouchingAssetIDs(value))
			os.Exit(1)
		}
		received++
		actions := orderTouchingActions(value)
		parts := make([]string, 0, len(actions))
		for _, action := range actions {
			parts = append(parts, action.Type+":"+strings.Join(action.AssetIDs, "|"))
		}
		sort.Strings(matches)
		fmt.Printf("message %d/%d: expected_asset_matches=%v order_touching=[%s] bytes=%d\n", received, *maxMessages, matches, strings.Join(parts, ", "), len(raw))
		if *printRaw {
			fmt.Println(raw)
		}
		if received >= *maxMessages {
			fmt.Printf("PASS: received %d raw mempool message(s)\n", received)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "FAILED: stream ended after %d message(s)\n", received)
	os.Exit(1)
}
