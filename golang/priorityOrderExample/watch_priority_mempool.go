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

const (
	defaultGrpcEndpoint = "your-endpoint.hype-mainnet.quiknode.pro:10000"
	defaultAuthToken    = "YOUR_QUICKNODE_TOKEN"
)

var zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

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

func decompress(data []byte) (string, error) {
	if len(data) >= 4 &&
		data[0] == zstdMagic[0] &&
		data[1] == zstdMagic[1] &&
		data[2] == zstdMagic[2] &&
		data[3] == zstdMagic[3] {
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

func priorityFees(value interface{}) []string {
	fees := []string{}
	switch v := value.(type) {
	case map[string]interface{}:
		if _, ok := v["source"]; ok {
			if eventType, _ := v["type"].(string); eventType == "order" && v["p"] != nil {
				fees = append(fees, fmt.Sprint(v["p"]))
			}
		}
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
	includeConfirmed := flag.Bool("include-confirmed", false, "Also include confirmed ORDER_PRIORITY events from replica_cmds")
	rawMempoolFlag := flag.Bool("raw-mempool", false, "Subscribe to raw MEMPOOL_TXS and detect grouping.p locally")
	allMempool := flag.Bool("all-mempool", false, "With -raw-mempool, print all MEMPOOL_TXS messages, not only priority transactions")
	maxMessages := flag.Int("max-messages", 0, "Stop after printing this many matching messages")
	compact := flag.Bool("compact", false, "Print compact payload text instead of pretty JSON")
	flag.Parse()
	rawMempool := *rawMempoolFlag || *allMempool

	endpoint := envOrDefault("GRPC_ENDPOINT", defaultGrpcEndpoint)
	authToken := envOrDefault("AUTH_TOKEN", envOrDefault("QN_AUTH_TOKEN", defaultAuthToken))
	if endpoint == defaultGrpcEndpoint {
		log.Fatal("Set GRPC_ENDPOINT to your QuickNode Hyperliquid mainnet or testnet gRPC endpoint")
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

	streamType := pb.StreamType_ORDER_PRIORITY
	filters := map[string]*pb.FilterValues{
		"source": {Values: []string{"mempool_txs"}},
	}
	if rawMempool {
		streamType = pb.StreamType_MEMPOOL_TXS
		filters = map[string]*pb.FilterValues{}
	} else if *includeConfirmed {
		filters = map[string]*pb.FilterValues{}
	}

	if err := stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Subscribe{
			Subscribe: &pb.StreamSubscribe{
				StreamType: streamType,
				StartBlock: *startBlock,
				Filters:    filters,
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
				timestamp := time.Now().UnixMilli()
				fmt.Printf("PING timestamp=%d\n", timestamp)
				_ = stream.Send(&pb.SubscribeRequest{
					Request: &pb.SubscribeRequest_Ping{
						Ping: &pb.Ping{Timestamp: timestamp},
					},
				})
			}
		}
	}()

	if rawMempool {
		fmt.Println("Watching raw MEMPOOL_TXS")
	} else if *includeConfirmed {
		fmt.Println("Watching ORDER_PRIORITY events from mempool_txs and replica_cmds")
	} else {
		fmt.Println("Watching pre-consensus ORDER_PRIORITY mempool events")
	}
	fmt.Printf("Endpoint: %s\n", endpoint)
	if !rawMempool && !*includeConfirmed {
		fmt.Println("Server filter: source=mempool_txs (not finalized)")
	} else if rawMempool && !*allMempool {
		fmt.Println("Local filter: priority grouping only")
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
		if pong := resp.GetPong(); pong != nil {
			fmt.Printf("PONG timestamp=%d\n", pong.Timestamp)
			continue
		}

		dataUpdate, ok := resp.Update.(*pb.SubscribeUpdate_Data)
		if !ok {
			continue
		}

		text, err := decompress([]byte(dataUpdate.Data.Data))
		if err != nil {
			log.Printf("decompress error at block %d: %v", dataUpdate.Data.BlockNumber, err)
			continue
		}
		if !matchesTextFilters(text, contains) {
			continue
		}

		var parsed interface{}
		parsedOK := json.Unmarshal([]byte(text), &parsed) == nil
		fees := []string{}
		if parsedOK {
			fees = priorityFees(parsed)
		}
		if rawMempool && !*allMempool && len(fees) == 0 {
			continue
		}

		printed++
		fmt.Printf("\nBlock %d | Timestamp %d\n", dataUpdate.Data.BlockNumber, dataUpdate.Data.Timestamp)
		if len(fees) > 0 {
			fmt.Printf("Priority p: %s\n", strings.Join(fees, ", "))
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
