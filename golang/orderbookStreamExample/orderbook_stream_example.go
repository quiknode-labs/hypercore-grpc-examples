// Orderbook Stream Example - Stream orderbook data via QuickNode gRPC
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/example/hyperliquid-grpc/proto"
)

// Mainnet: "your-endpoint.hype-mainnet.quiknode.pro:10000"
// Testnet: "your-endpoint.hype-testnet.quiknode.pro:10000"
const (
	defaultGrpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	defaultAuthToken    = "your-quicknode-token"
	maxRetries          = 10
	baseDelay           = 2 * time.Second
)

var (
	grpcEndpoint = envOrDefault("GRPC_ENDPOINT", defaultGrpcEndpoint)
	authToken    = envOrDefault("AUTH_TOKEN", envOrDefault("QN_AUTH_TOKEN", defaultAuthToken))
)

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func splitCoins(coinArg string, all bool) []string {
	if all {
		return []string{}
	}
	parts := strings.Split(coinArg, ",")
	coins := make([]string, 0, len(parts))
	for _, part := range parts {
		coin := strings.TrimSpace(part)
		if coin != "" {
			coins = append(coins, coin)
		}
	}
	return coins
}

func connectOrderBookClient() (*grpc.ClientConn, pb.OrderBookStreamingClient, context.Context, error) {
	creds := credentials.NewClientTLSFromCert(nil, "")
	conn, err := grpc.Dial(grpcEndpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)))
	if err != nil {
		return nil, nil, nil, err
	}
	client := pb.NewOrderBookStreamingClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", authToken)
	return conn, client, ctx, nil
}

func levelText(level *pb.L2Level) string {
	if level == nil || level.Px == "" {
		return "n/a"
	}
	return fmt.Sprintf("%s / %s (%d)", level.Px, level.Sz, level.N)
}

func streamL2Orderbook(coin string, nLevels uint32, nSigFigs *uint32, mantissa *uint64, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming L2 Orderbook for %s\n", coin)
	fmt.Printf("Levels: %d\n", nLevels)
	if nSigFigs != nil {
		fmt.Printf("Sig Figs: %d\n", *nSigFigs)
	}
	if mantissa != nil {
		fmt.Printf("Mantissa: %d\n", *mantissa)
	}
	fmt.Println("Auto-reconnect: true")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	totalMsgCount := 0

	for retryCount < maxRetries {
		creds := credentials.NewClientTLSFromCert(nil, "")
		conn, err := grpc.Dial(grpcEndpoint,
			grpc.WithTransportCredentials(creds),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		client := pb.NewOrderBookStreamingClient(conn)
		ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", authToken)

		request := &pb.L2BookRequest{
			Coin:     coin,
			NLevels:  nLevels,
			NSigFigs: nSigFigs,
			Mantissa: mantissa,
		}

		if retryCount > 0 {
			fmt.Printf("\n🔄 Reconnecting (attempt %d/%d)...\n", retryCount+1, maxRetries)
		} else {
			fmt.Printf("Connecting to %s...\n", grpcEndpoint)
		}

		stream, err := client.StreamL2Book(ctx, request)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start stream: %w", err)
		}

		shouldRetry := false

		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					fmt.Printf("\n⚠️  Server reinitialized: %s\n", st.Message())
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("⏳ Waiting %v before reconnecting...\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					} else {
						fmt.Printf("\n❌ Max retries (%d) reached. Giving up.\n", maxRetries)
						conn.Close()
						return nil
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			totalMsgCount++
			if totalMsgCount == 1 {
				fmt.Println("✓ First L2 update received!")
				fmt.Println()
				retryCount = 0 // Reset on success
			}

			// Display orderbook
			fmt.Println("\n" + strings.Repeat("─", 60))
			fmt.Printf("Block: %d | Time: %d | Coin: %s\n", update.BlockNumber, update.Time, update.Coin)
			fmt.Println(strings.Repeat("─", 60))

			// Display asks (reversed)
			if len(update.Asks) > 0 {
				fmt.Println("\n  ASKS:")
				askCount := len(update.Asks)
				if askCount > 10 {
					askCount = 10
				}
				for i := askCount - 1; i >= 0; i-- {
					level := update.Asks[i]
					fmt.Printf("    %12s | %12s | (%d orders)\n", level.Px, level.Sz, level.N)
				}
			}

			// Display spread
			if len(update.Bids) > 0 && len(update.Asks) > 0 {
				// Simple spread calculation (string to float conversion omitted for brevity)
				fmt.Println("\n  " + strings.Repeat("─", 44))
				fmt.Printf("  SPREAD: (best bid: %s, best ask: %s)\n", update.Bids[0].Px, update.Asks[0].Px)
				fmt.Println("  " + strings.Repeat("─", 44))
			}

			// Display bids
			if len(update.Bids) > 0 {
				fmt.Println("\n  BIDS:")
				bidCount := len(update.Bids)
				if bidCount > 10 {
					bidCount = 10
				}
				for i := 0; i < bidCount; i++ {
					level := update.Bids[i]
					fmt.Printf("    %12s | %12s | (%d orders)\n", level.Px, level.Sz, level.N)
				}
			}

			fmt.Printf("\n  Messages received: %d\n", totalMsgCount)
			if maxMessages > 0 && totalMsgCount >= maxMessages {
				fmt.Printf("\nReached max messages (%d), stopping...\n", maxMessages)
				conn.Close()
				return nil
			}
		}

		conn.Close()

		if !shouldRetry {
			break
		}
	}

	return nil
}

func streamBbo(coins []string, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming BBO for %s\n", strings.Join(coins, ","))
	if len(coins) == 0 {
		fmt.Println("Streaming BBO for all eligible coins")
	}
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	msgCount := 0

	for retryCount < maxRetries {
		conn, client, ctx, err := connectOrderBookClient()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		stream, err := client.StreamBboBook(ctx, &pb.BboBookRequest{Coins: coins})
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start BBO stream: %w", err)
		}

		shouldRetry := false
		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("DATA_LOSS from BBO stream; reconnecting in %v\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			msgCount++
			retryCount = 0
			fmt.Printf("[%d] BBO %s block=%d bid=%s ask=%s\n",
				msgCount, update.Coin, update.BlockNumber, levelText(update.Bid), levelText(update.Ask))

			if maxMessages > 0 && msgCount >= maxMessages {
				conn.Close()
				return nil
			}
		}

		conn.Close()
		if !shouldRetry {
			break
		}
	}

	return nil
}

func streamL2BookDiff(coins []string, nLevels uint32, nSigFigs *uint32, mantissa *uint64, skipInitialSnapshot bool, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming L2 Book Diffs for %s\n", strings.Join(coins, ","))
	if len(coins) == 0 {
		fmt.Println("Streaming L2 Book Diffs for all eligible coins")
	}
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	msgCount := 0

	for retryCount < maxRetries {
		conn, client, ctx, err := connectOrderBookClient()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		request := &pb.L2BookDiffRequest{
			Coins:               coins,
			NLevels:             nLevels,
			NSigFigs:            nSigFigs,
			Mantissa:            mantissa,
			SkipInitialSnapshot: skipInitialSnapshot,
		}
		stream, err := client.StreamL2BookDiff(ctx, request)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start L2 diff stream: %w", err)
		}

		shouldRetry := false
		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("DATA_LOSS from L2 diff stream; reconnecting in %v\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			msgCount++
			retryCount = 0
			fmt.Printf("[%d] L2 diff height=%d snapshot=%t coins=%d\n", msgCount, update.Height, update.Snapshot, len(update.Diffs))
			for i, diff := range update.Diffs {
				if i >= 5 {
					break
				}
				fmt.Printf("  %s seq=%d prev_seq=%d snapshot=%t bid_changes=%d ask_changes=%d\n",
					diff.Coin, diff.Seq, diff.PrevSeq, diff.Snapshot, len(diff.Bids), len(diff.Asks))
			}

			if maxMessages > 0 && msgCount >= maxMessages {
				conn.Close()
				return nil
			}
		}

		conn.Close()
		if !shouldRetry {
			break
		}
	}

	return nil
}

func streamL4BookUpdates(coins []string, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming L4 Book Updates for %s\n", strings.Join(coins, ","))
	if len(coins) == 0 {
		fmt.Println("Streaming L4 Book Updates for all eligible coins")
	}
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	msgCount := 0

	for retryCount < maxRetries {
		conn, client, ctx, err := connectOrderBookClient()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		stream, err := client.StreamL4BookUpdates(ctx, &pb.L4BookUpdatesRequest{Coins: coins})
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start L4 updates stream: %w", err)
		}

		shouldRetry := false
		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("DATA_LOSS from L4 updates stream; reconnecting in %v\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			msgCount++
			retryCount = 0
			fmt.Printf("[%d] L4 updates height=%d snapshot=%t diffs=%d\n", msgCount, update.Height, update.Snapshot, len(update.Diffs))
			for i, diff := range update.Diffs {
				if i >= 5 {
					break
				}
				fmt.Printf("  %s %s oid=%d side=%s px=%s sz=%s\n",
					diff.DiffType.String(), diff.Coin, diff.Oid, diff.Side, diff.Px, diff.Sz)
			}

			if maxMessages > 0 && msgCount >= maxMessages {
				conn.Close()
				return nil
			}
		}

		conn.Close()
		if !shouldRetry {
			break
		}
	}

	return nil
}

func streamTpslUpdates(coins []string, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming TP/SL Updates for %s\n", strings.Join(coins, ","))
	if len(coins) == 0 {
		fmt.Println("Streaming TP/SL Updates for all perp coins")
	}
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	msgCount := 0

	for retryCount < maxRetries {
		conn, client, ctx, err := connectOrderBookClient()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		stream, err := client.StreamTpslUpdates(ctx, &pb.TpslUpdatesRequest{Coins: coins})
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start TP/SL updates stream: %w", err)
		}

		shouldRetry := false
		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("DATA_LOSS from TP/SL updates stream; reconnecting in %v\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			msgCount++
			retryCount = 0
			fmt.Printf("[%d] TP/SL height=%d snapshot=%t diffs=%d\n", msgCount, update.Height, update.Snapshot, len(update.Diffs))
			for i, diff := range update.Diffs {
				if i >= 5 {
					break
				}
				fmt.Printf("  %s %s oid=%d trigger=%s limit=%s sz=%s reason=%s\n",
					diff.DiffType.String(), diff.Coin, diff.Oid, diff.TriggerPx, diff.LimitPx, diff.Sz, diff.Reason)
			}

			if maxMessages > 0 && msgCount >= maxMessages {
				conn.Close()
				return nil
			}
		}

		conn.Close()
		if !shouldRetry {
			break
		}
	}

	return nil
}

func streamL4Orderbook(coin string, maxMessages int) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming L4 Orderbook for %s\n", coin)
	fmt.Println("Auto-reconnect: true")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0
	totalMsgCount := 0

	for retryCount < maxRetries {
		creds := credentials.NewClientTLSFromCert(nil, "")
		conn, err := grpc.Dial(grpcEndpoint,
			grpc.WithTransportCredentials(creds),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		client := pb.NewOrderBookStreamingClient(conn)
		ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", authToken)

		request := &pb.L4BookRequest{
			Coin: coin,
		}

		if retryCount > 0 {
			fmt.Printf("\n🔄 Reconnecting (attempt %d/%d)...\n", retryCount+1, maxRetries)
		} else {
			fmt.Printf("Connecting to %s...\n", grpcEndpoint)
		}

		stream, err := client.StreamL4Book(ctx, request)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to start stream: %w", err)
		}

		snapshotReceived := false
		shouldRetry := false

		for {
			update, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.DataLoss {
					fmt.Printf("\n⚠️  Server reinitialized: %s\n", st.Message())
					retryCount++
					if retryCount < maxRetries {
						delay := baseDelay * time.Duration(math.Pow(2, float64(retryCount-1)))
						fmt.Printf("⏳ Waiting %v before reconnecting...\n", delay)
						time.Sleep(delay)
						shouldRetry = true
						break
					} else {
						fmt.Printf("\n❌ Max retries (%d) reached. Giving up.\n", maxRetries)
						conn.Close()
						return nil
					}
				}
				conn.Close()
				return fmt.Errorf("stream error: %w", err)
			}

			totalMsgCount++

			if snapshot := update.GetSnapshot(); snapshot != nil {
				snapshotReceived = true
				retryCount = 0 // Reset on success

				fmt.Println("\n✓ L4 Snapshot Received!")
				fmt.Println(strings.Repeat("─", 60))
				fmt.Printf("Coin: %s\n", snapshot.Coin)
				fmt.Printf("Height: %d\n", snapshot.Height)
				fmt.Printf("Time: %d\n", snapshot.Time)
				fmt.Printf("Bids: %d orders\n", len(snapshot.Bids))
				fmt.Printf("Asks: %d orders\n", len(snapshot.Asks))
				fmt.Println(strings.Repeat("─", 60))

				// Sample bids
				if len(snapshot.Bids) > 0 {
					fmt.Println("\nSample Bids (first 5):")
					bidCount := len(snapshot.Bids)
					if bidCount > 5 {
						bidCount = 5
					}
					for i := 0; i < bidCount; i++ {
						order := snapshot.Bids[i]
						userShort := order.User
						if len(userShort) > 10 {
							userShort = userShort[:10] + "..."
						}
						fmt.Printf("  OID: %d | Price: %s | Size: %s | User: %s\n",
							order.Oid, order.LimitPx, order.Sz, userShort)
					}
				}

				// Sample asks
				if len(snapshot.Asks) > 0 {
					fmt.Println("\nSample Asks (first 5):")
					askCount := len(snapshot.Asks)
					if askCount > 5 {
						askCount = 5
					}
					for i := 0; i < askCount; i++ {
						order := snapshot.Asks[i]
						userShort := order.User
						if len(userShort) > 10 {
							userShort = userShort[:10] + "..."
						}
						fmt.Printf("  OID: %d | Price: %s | Size: %s | User: %s\n",
							order.Oid, order.LimitPx, order.Sz, userShort)
					}
				}

			} else if diff := update.GetDiff(); diff != nil {
				if !snapshotReceived {
					fmt.Println("\n⚠ Received diff before snapshot")
				}

				var diffData map[string]interface{}
				if err := json.Unmarshal([]byte(diff.Data), &diffData); err == nil {
					orderStatuses := []interface{}{}
					bookDiffs := []interface{}{}

					if os, ok := diffData["order_statuses"].([]interface{}); ok {
						orderStatuses = os
					}
					if bd, ok := diffData["book_diffs"].([]interface{}); ok {
						bookDiffs = bd
					}

					fmt.Printf("\n[Block %d] L4 Diff:\n", diff.Height)
					fmt.Printf("  Time: %d\n", diff.Time)
					fmt.Printf("  Order Statuses: %d\n", len(orderStatuses))
					fmt.Printf("  Book Diffs: %d\n", len(bookDiffs))

					if len(bookDiffs) > 0 && len(bookDiffs) <= 5 {
						pretty, _ := json.MarshalIndent(bookDiffs, "  ", "  ")
						fmt.Printf("  Diffs: %s\n", pretty)
					}
				}
			}

			if maxMessages > 0 && totalMsgCount >= maxMessages {
				fmt.Printf("\nReached max messages (%d), stopping...\n", maxMessages)
				conn.Close()
				return nil
			}
		}

		conn.Close()

		if !shouldRetry {
			break
		}
	}

	return nil
}

func main() {
	mode := flag.String("mode", "bbo", "Streaming mode: l2, l4, bbo, l2-diff, l4-updates, or tpsl")
	coin := flag.String("coin", "BTC", "Coin symbol or comma-separated symbols to stream")
	allCoins := flag.Bool("all", false, "Subscribe to all eligible coins on multi-coin streams")
	levels := flag.Uint("levels", 20, "Number of price levels for L2")
	sigFigs := flag.Uint("sig-figs", 0, "Significant figures for L2 price bucketing (2-5, 0 = disabled)")
	mantissaFlag := flag.Uint64("mantissa", 0, "Mantissa for L2 price bucketing (1, 2, or 5, 0 = disabled)")
	skipInitialSnapshot := flag.Bool("skip-initial-snapshot", false, "For l2-diff, skip the initial snapshot")
	maxMessages := flag.Int("max-messages", 0, "Maximum messages to receive (0 = unlimited)")

	flag.Parse()

	if *allCoins && (*mode == "l2" || *mode == "l4") {
		fmt.Fprintln(os.Stderr, "-all is only supported for bbo, l2-diff, l4-updates, and tpsl. Use -coin for l2 or l4.")
		os.Exit(2)
	}
	coins := splitCoins(*coin, *allCoins)
	if !*allCoins && len(coins) == 0 {
		fmt.Fprintln(os.Stderr, "-coin must include at least one symbol. Use -all to subscribe to every eligible coin on multi-coin streams.")
		os.Exit(2)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Hyperliquid Orderbook Stream Example")
	fmt.Printf("Endpoint: %s\n", grpcEndpoint)
	fmt.Println(strings.Repeat("=", 60))

	if authToken == defaultAuthToken {
		log.Fatal("Set AUTH_TOKEN to your QuickNode token before running this example")
	}

	// Convert optional flags to pointers (nil if not set)
	var nSigFigs *uint32
	if *sigFigs > 0 {
		v := uint32(*sigFigs)
		nSigFigs = &v
	}
	var mantissaVal *uint64
	if *mantissaFlag > 0 {
		mantissaVal = mantissaFlag
	}

	var err error
	singleCoin := ""
	if len(coins) > 0 {
		singleCoin = coins[0]
	}
	if *mode == "l2" {
		err = streamL2Orderbook(singleCoin, uint32(*levels), nSigFigs, mantissaVal, *maxMessages)
	} else if *mode == "l4" {
		err = streamL4Orderbook(singleCoin, *maxMessages)
	} else if *mode == "bbo" {
		err = streamBbo(coins, *maxMessages)
	} else if *mode == "l2-diff" {
		err = streamL2BookDiff(coins, uint32(*levels), nSigFigs, mantissaVal, *skipInitialSnapshot, *maxMessages)
	} else if *mode == "l4-updates" {
		err = streamL4BookUpdates(coins, *maxMessages)
	} else if *mode == "tpsl" {
		err = streamTpslUpdates(coins, *maxMessages)
	} else {
		log.Fatal("Invalid mode. Use -mode=l2, l4, bbo, l2-diff, l4-updates, or tpsl")
	}

	if err != nil {
		log.Fatal(err)
	}
}
