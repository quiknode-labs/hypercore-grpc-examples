package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	pb "github.com/example/hyperliquid-grpc/proto"
)

// Configuration
const (
	grpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	authToken    = "your-auth-token"
)

// Zstd magic number
var zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

func decompress(data []byte) (string, error) {
	if len(data) < 4 {
		return string(data), nil
	}

	// Check zstd magic number
	if data[0] == zstdMagic[0] && data[1] == zstdMagic[1] &&
		data[2] == zstdMagic[2] && data[3] == zstdMagic[3] {
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return "", err
		}
		defer decoder.Close()

		decompressed, err := decoder.DecodeAll(data, nil)
		if err != nil {
			return "", err
		}
		return string(decompressed), nil
	}

	return string(data), nil
}

func createConnection() (*grpc.ClientConn, error) {
	creds := credentials.NewClientTLSFromCert(nil, "")

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	return grpc.Dial(grpcEndpoint, opts...)
}

func streamData(ctx context.Context, streamType string, filters map[string][]string) error {
	conn, err := createConnection()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewStreamingClient(conn)

	// Add auth metadata
	ctx = metadata.AppendToOutgoingContext(ctx, "x-token", authToken)

	stream, err := client.StreamData(ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Build subscription request
	subscribe := &pb.StreamSubscribe{
		StreamType: pb.StreamType(pb.StreamType_value[streamType]),
		StartBlock: 0,
	}

	// Add filters if provided
	if len(filters) > 0 {
		subscribe.Filters = make(map[string]*pb.FilterValues)
		for field, values := range filters {
			subscribe.Filters[field] = &pb.FilterValues{Values: values}
		}
		log.Printf("Filters applied: %v", filters)
	}

	// Send subscription
	if err := stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Subscribe{Subscribe: subscribe},
	}); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Printf("Streaming %s...", streamType)

	// Keep-alive ping goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stream.Send(&pb.SubscribeRequest{
					Request: &pb.SubscribeRequest_Ping{
						Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()},
					},
				})
			}
		}
	}()

	// Receive data
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			log.Println("Stream ended")
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		switch u := resp.Update.(type) {
		case *pb.SubscribeUpdate_Data:
			decompressed, err := decompress(u.Data.Data)
			if err != nil {
				log.Printf("Decompress error: %v", err)
				continue
			}

			var parsed interface{}
			if err := json.Unmarshal([]byte(decompressed), &parsed); err == nil {
				prettyJSON, _ := json.MarshalIndent(parsed, "", "  ")
				fmt.Printf("\nBlock %d | Timestamp %d\n%s\n",
					u.Data.BlockNumber, u.Data.Timestamp, string(prettyJSON))
			} else {
				fmt.Printf("Block %d: %s\n", u.Data.BlockNumber, decompressed)
			}

		case *pb.SubscribeUpdate_Pong:
			log.Printf("Pong: %d", u.Pong.Timestamp)
		}
	}
}

func main() {
	streamType := flag.String("stream", "TRADES", "Stream type: TRADES, ORDERS, EVENTS, etc.")
	filterFlag := flag.String("filter", "", "Filters in format: field=val1,val2;field2=val3")
	flag.Parse()

	// Parse filters
	filters := make(map[string][]string)
	if *filterFlag != "" {
		for _, f := range strings.Split(*filterFlag, ";") {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				filters[parts[0]] = strings.Split(parts[1], ",")
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if err := streamData(ctx, strings.ToUpper(*streamType), filters); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
