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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	pb "github.com/example/hyperliquid-grpc/proto"
)

const (
	defaultGrpcEndpoint = "your-endpoint.hype-testnet.quiknode.pro:10000"
	defaultAuthToken    = "YOUR_QUICKNODE_TOKEN"
)

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func createConnection(endpoint string) (*grpc.ClientConn, error) {
	return grpc.Dial(
		endpoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
}

func priorityFees(value interface{}) []string {
	fees := []string{}
	switch v := value.(type) {
	case map[string]interface{}:
		if grouping, ok := v["grouping"].(map[string]interface{}); ok {
			if p, ok := grouping["p"]; ok {
				fees = append(fees, fmt.Sprint(p))
			}
		}
		for _, item := range v {
			fees = append(fees, priorityFees(item)...)
		}
	case []interface{}:
		for _, item := range v {
			fees = append(fees, priorityFees(item)...)
		}
	}
	return fees
}

func matchesTextFilters(text string, contains []string) bool {
	if len(contains) == 0 {
		return true
	}
	for _, needle := range contains {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func main() {
	startBlock := flag.Uint64("start-block", 0, "Start block for the stream")
	containsRaw := flag.String("contains", "", "Comma-separated text filters")
	allMempool := flag.Bool("all-mempool", false, "Print all MEMPOOL_TXS messages, not only priority transactions")
	maxMessages := flag.Int("max-messages", 0, "Stop after printing this many matching messages")
	compact := flag.Bool("compact", false, "Print compact payload text instead of pretty JSON")
	flag.Parse()

	endpoint := envOrDefault("GRPC_ENDPOINT", defaultGrpcEndpoint)
	authToken := envOrDefault("AUTH_TOKEN", envOrDefault("QN_AUTH_TOKEN", defaultAuthToken))
	if endpoint == defaultGrpcEndpoint {
		log.Fatal("Set GRPC_ENDPOINT to your QuickNode Hyperliquid testnet gRPC endpoint")
	}
	if authToken == defaultAuthToken {
		log.Fatal("Set AUTH_TOKEN to your QuickNode token")
	}

	contains := []string{}
	if *containsRaw != "" {
		for _, part := range strings.Split(*containsRaw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				contains = append(contains, part)
			}
		}
	}

	conn, err := createConnection(endpoint)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-token", authToken)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	client := pb.NewStreamingClient(conn)
	stream, err := client.StreamData(ctx)
	if err != nil {
		log.Fatalf("failed to create stream: %v", err)
	}

	if err := stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Subscribe{
			Subscribe: &pb.StreamSubscribe{
				StreamType: pb.StreamType_MEMPOOL_TXS,
				StartBlock: *startBlock,
			},
		},
	}); err != nil {
		log.Fatalf("failed to subscribe: %v", err)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = stream.Send(&pb.SubscribeRequest{
					Request: &pb.SubscribeRequest_Ping{
						Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()},
					},
				})
			}
		}
	}()

	fmt.Println("Watching testnet MEMPOOL_TXS")
	fmt.Printf("Endpoint: %s\n", endpoint)
	if !*allMempool {
		fmt.Println("Filter: priority grouping only")
	}
	if len(contains) > 0 {
		fmt.Printf("Text filters: %v\n", contains)
	}

	printed := 0
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Fatalf("receive error: %v", err)
		}

		dataUpdate, ok := resp.Update.(*pb.SubscribeUpdate_Data)
		if !ok {
			continue
		}

		text := dataUpdate.Data.Data
		if !matchesTextFilters(text, contains) {
			continue
		}

		var parsed interface{}
		parsedOK := json.Unmarshal([]byte(text), &parsed) == nil
		fees := []string{}
		if parsedOK {
			fees = priorityFees(parsed)
		}
		if !*allMempool && len(fees) == 0 {
			continue
		}

		printed++
		fmt.Printf("\nBlock %d | Timestamp %d\n", dataUpdate.Data.BlockNumber, dataUpdate.Data.Timestamp)
		if len(fees) > 0 {
			fmt.Printf("Priority fee grouping p: %s\n", strings.Join(fees, ", "))
		}
		if *compact {
			if len(text) > 1000 {
				fmt.Println(text[:1000])
			} else {
				fmt.Println(text)
			}
		} else if parsedOK {
			pretty, _ := json.MarshalIndent(parsed, "", "  ")
			fmt.Println(string(pretty))
		} else {
			fmt.Println(text)
		}

		if *maxMessages > 0 && printed >= *maxMessages {
			return
		}
	}
}
