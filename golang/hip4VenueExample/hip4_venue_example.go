// HIP-4 Outcome Markets Example - Stream one venue's orders with
// subscription tagging and signer enrichment.
//
// Demonstrates the three HIP-4-era streaming features:
//  1. "venue" filter key: server-side expansion of an outcome venue's name
//     to its full coin set (coins look like "#146870" and churn as outcomes
//     settle - the server tracks that for you).
//  2. subscription_id: a client-chosen tag echoed on every update, plus the
//     stream_type field, so multiplexed subscriptions are distinguishable.
//  3. EnrichmentOptions.include_signer: each order carries "signer" - the
//     wallet that actually SUBMITTED it (master or approved API wallet),
//     recovered from the action's signature. Unsigned engine events
//     (trigger fires, liquidations, TWAP children) carry "signer": null.
//
// Find active venue names via the info endpoint: {"type":"outcomeMeta"}
// (fields: outcomes[].venue, deployers[].venue).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/example/hyperliquid-grpc/proto"
)

// HIP-4 launches on testnet first; use your testnet endpoint until mainnet
// venues go live.
// Mainnet: "your-endpoint.hype-mainnet.quiknode.pro:10000"
// Testnet: "your-endpoint.hype-testnet.quiknode.pro:10000"
const (
	grpcEndpoint = "your-endpoint.hype-testnet.quiknode.pro:10000"
	authToken    = "your-auth-token"
	venueName    = "txyz" // an active venue from {"type":"outcomeMeta"}
)

func decompress(data []byte) (string, error) {
	if len(data) >= 4 && data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD {
		decoder, _ := zstd.NewReader(nil)
		defer decoder.Close()
		decompressed, err := decoder.DecodeAll(data, nil)
		if err != nil {
			return "", err
		}
		return string(decompressed), nil
	}
	return string(data), nil
}

func streamVenueOrders() error {
	creds := credentials.NewClientTLSFromCert(nil, "")
	conn, err := grpc.Dial(grpcEndpoint, grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewStreamingClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", authToken)

	stream, err := client.StreamData(ctx)
	if err != nil {
		return err
	}

	// Subscribe to ORDERS for one outcome venue, tagged and signer-enriched.
	subscribe := &pb.StreamSubscribe{
		StreamType: pb.StreamType_ORDERS,
		StartBlock: 0,
		FilterName: "hip4-" + venueName,
		Filters: map[string]*pb.FilterValues{
			// Reserved key: expanded server-side to the venue's coin set.
			// Also accepted: "venues", "deployer", "deployers" (address).
			"venue": {Values: []string{venueName}},
		},
		// Echoed on every update for this stream type.
		SubscriptionId: "hip4-orders-demo",
		// Adds "signer" to each order (requires a server with signer
		// enrichment enabled; testnet has it on).
		Enrichment: &pb.EnrichmentOptions{IncludeSigner: true},
	}

	if err := stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Subscribe{Subscribe: subscribe},
	}); err != nil {
		return err
	}

	log.Printf("Streaming ORDERS for venue %q with signer enrichment", venueName)

	// Keep-alive pings
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ping := &pb.SubscribeRequest{
				Request: &pb.SubscribeRequest_Ping{
					Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()},
				},
			}
			if err := stream.Send(ping); err != nil {
				return
			}
		}
	}()

	for {
		update, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		data := update.GetData()
		if data == nil {
			continue // pong
		}

		payload, err := decompress([]byte(data.Data))
		if err != nil {
			log.Printf("decompress: %v", err)
			continue
		}

		// Every update says which subscription it belongs to.
		fmt.Printf("[block %d] streamType=%s subscriptionId=%q\n",
			data.BlockNumber, data.StreamType, data.SubscriptionId)

		var orders []map[string]any
		if err := json.Unmarshal([]byte(payload), &orders); err != nil {
			fmt.Println(payload)
			continue
		}
		for _, entry := range orders {
			order, _ := entry["order"].(map[string]any)
			if order == nil {
				continue
			}
			inner, _ := order["order"].(map[string]any)
			coin := ""
			user := ""
			if inner != nil {
				coin, _ = inner["coin"].(string)
			}
			user, _ = order["user"].(string)
			// "signer" is present because of EnrichmentOptions above.
			signer := entry["signer"]
			fmt.Printf("  coin=%s user=%v signer=%v status=%v\n",
				coin, user, signer, entry["status"])
		}
	}
}

func main() {
	if err := streamVenueOrders(); err != nil {
		log.Fatal(err)
	}
}
