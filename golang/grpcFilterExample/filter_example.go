// Filtering Example - Stream only trades for specific coins
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

const (
	grpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	authToken    = "your-auth-token"
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

func streamWithFilter() error {
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

	// Subscribe to TRADES with filters
	subscribe := &pb.StreamSubscribe{
		StreamType: pb.StreamType_TRADES,
		StartBlock: 0,
		FilterName: "eth-btc-trades",
		Filters: map[string]*pb.FilterValues{
			// Filter for specific coins only
			"coin": {Values: []string{"ETH", "BTC"}},
		},
	}

	if err := stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Subscribe{Subscribe: subscribe},
	}); err != nil {
		return err
	}

	log.Println("Streaming TRADES filtered by coin: ETH, BTC")

	// Keep-alive pings
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stream.Send(&pb.SubscribeRequest{
				Request: &pb.SubscribeRequest_Ping{Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()}},
			})
		}
	}()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if data := resp.GetData(); data != nil {
			decompressed, _ := decompress(data.Data)
			var parsed interface{}
			if json.Unmarshal([]byte(decompressed), &parsed) == nil {
				pretty, _ := json.MarshalIndent(parsed, "", "  ")
				fmt.Printf("Block %d:\n%s\n", data.BlockNumber, pretty)
			}
		}
	}
}

func main() {
	if err := streamWithFilter(); err != nil {
		log.Fatal(err)
	}
}
