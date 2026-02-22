// Orderbook Stream Example - Stream L2 and L4 orderbook data via gRPC
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/example/hyperliquid-grpc/proto"
)

const (
	grpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	authToken    = "your-auth-token"
	maxRetries   = 10
	baseDelay    = 2 * time.Second
)

func streamL2Orderbook(coin string, nLevels uint32) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Streaming L2 Orderbook for %s\n", coin)
	fmt.Printf("Levels: %d\n", nLevels)
	fmt.Println("Auto-reconnect: true")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	retryCount := 0

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
			Coin:    coin,
			NLevels: nLevels,
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

		msgCount := 0
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

			msgCount++
			if msgCount == 1 {
				fmt.Println("✓ First L2 update received!\n")
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

			fmt.Printf("\n  Messages received: %d\n", msgCount)
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
	mode := flag.String("mode", "l2", "Streaming mode: l2 or l4")
	coin := flag.String("coin", "BTC", "Coin symbol to stream")
	levels := flag.Uint("levels", 20, "Number of price levels for L2")
	maxMessages := flag.Int("max-messages", 0, "Maximum messages for L4 (0 = unlimited)")

	flag.Parse()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Hyperliquid Orderbook Stream Example")
	fmt.Printf("Endpoint: %s\n", grpcEndpoint)
	fmt.Println(strings.Repeat("=", 60))

	var err error
	if *mode == "l2" {
		err = streamL2Orderbook(*coin, uint32(*levels))
	} else if *mode == "l4" {
		err = streamL4Orderbook(*coin, *maxMessages)
	} else {
		log.Fatal("Invalid mode. Use -mode=l2 or -mode=l4")
	}

	if err != nil {
		log.Fatal(err)
	}
}
