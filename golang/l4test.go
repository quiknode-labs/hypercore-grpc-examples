// Quick L4 orderbook stream test - confirms gRPC connection and snapshot receipt
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	grpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	authToken    = "your-auth-token"
)

func main() {
	coin := "BTC"
	fmt.Printf("Testing L4 orderbook stream for %s (NO timeout on context)\n", coin)

	creds := credentials.NewClientTLSFromCert(nil, "")
	conn, err := grpc.Dial(grpcEndpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewOrderBookStreamingClient(conn)

	// NO timeout - just like the working example
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", authToken)

	stream, err := client.StreamL4Book(ctx, &pb.L4BookRequest{Coin: coin})
	if err != nil {
		log.Fatalf("failed to start stream: %v", err)
	}

	fmt.Println("Connected, waiting for data...")
	start := time.Now()
	msgCount := 0

	for msgCount < 5 {
		update, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("Stream ended (EOF)")
			break
		}
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.DataLoss {
				fmt.Printf("Server reinitialized: %s\n", st.Message())
				break
			}
			log.Fatalf("stream error: %v", err)
		}

		msgCount++
		elapsed := time.Since(start)

		if snapshot := update.GetSnapshot(); snapshot != nil {
			fmt.Printf("\n[%v] SNAPSHOT received!\n", elapsed.Round(time.Millisecond))
			fmt.Printf("  Coin: %s\n", snapshot.Coin)
			fmt.Printf("  Height: %d\n", snapshot.Height)
			fmt.Printf("  Time: %d\n", snapshot.Time)
			fmt.Printf("  Bids: %d orders\n", len(snapshot.Bids))
			fmt.Printf("  Asks: %d orders\n", len(snapshot.Asks))

			if len(snapshot.Bids) > 0 {
				o := snapshot.Bids[0]
				fmt.Printf("  Best bid: %s @ %s (oid=%d)\n", o.Sz, o.LimitPx, o.Oid)
			}
			if len(snapshot.Asks) > 0 {
				o := snapshot.Asks[0]
				fmt.Printf("  Best ask: %s @ %s (oid=%d)\n", o.Sz, o.LimitPx, o.Oid)
			}
		} else if diff := update.GetDiff(); diff != nil {
			var diffData map[string]interface{}
			json.Unmarshal([]byte(diff.Data), &diffData)
			osCount := 0
			bdCount := 0
			if os, ok := diffData["order_statuses"].([]interface{}); ok {
				osCount = len(os)
			}
			if bd, ok := diffData["book_diffs"].([]interface{}); ok {
				bdCount = len(bd)
			}
			fmt.Printf("[%v] DIFF block=%d order_statuses=%d book_diffs=%d\n",
				elapsed.Round(time.Millisecond), diff.Height, osCount, bdCount)
		}
	}

	fmt.Printf("\nDone. Received %d messages in %v\n", msgCount, time.Since(start).Round(time.Millisecond))
}
