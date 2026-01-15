/*
S3 Blocks Backfill Example
==========================

WHY THIS EXISTS:
----------------
The `blocks` stream contains raw HyperLiquid replica commands - the fundamental
building blocks of the chain. Unlike other streams (trades, orders, book_updates),
blocks are ONLY available via gRPC streaming.

The problem: gRPC connections can disconnect. When that happens, you miss blocks.
There's no HTTP API to query historical blocks.

The solution: The Hyperliquid Foundation maintains a public S3 bucket with
historical data. You can backfill missed blocks directly from there.


S3 BUCKET STRUCTURE:
--------------------
Bucket: s3://hl-mainnet-node-data/
Access: Requester pays (you pay for data transfer)

Available prefixes:
  - replica_cmds/          <- blocks data

replica_cmds/ directory structure:
  replica_cmds/{CHECKPOINT_TIMESTAMP}/{DATE}/{BLOCK_RANGE_FILE}

  Example: replica_cmds/1704067200/20240101/830000000-830010000


FILE FORMAT:
------------
- JSON Lines format (one JSON object per line)
- NO block_number field in the JSON (ordering is implicit by line position)
- Files are MASSIVE: 3-7 GB each (uncompressed)


USAGE:
------
go run s3_blocks_backfill.go


COST CONSIDERATIONS:
--------------------
- Requester pays bucket - you pay for data transfer
- Files are 3-7 GB each
- Stream instead of downloading entirely when possible
*/
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	S3Bucket     = "hl-mainnet-node-data"
	BlocksPrefix = "replica_cmds"
)

// BlockRange represents a block range file in S3
type BlockRange struct {
	Checkpoint string
	Date       string
	StartBlock int64
	EndBlock   int64
	S3Key      string
}

// ParseBlockRange parses S3 key: replica_cmds/1704067200/20240101/830000000-830010000
func ParseBlockRange(key string) *BlockRange {
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != BlocksPrefix {
		return nil
	}

	rangeParts := strings.Split(parts[3], "-")
	if len(rangeParts) != 2 {
		return nil
	}

	start, err1 := strconv.ParseInt(rangeParts[0], 10, 64)
	end, err2 := strconv.ParseInt(rangeParts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return nil
	}

	return &BlockRange{
		Checkpoint: parts[1],
		Date:       parts[2],
		StartBlock: start,
		EndBlock:   end,
		S3Key:      key,
	}
}

// Block represents a parsed block from S3
type Block struct {
	BlockNumber int64
	Data        map[string]interface{}
}

func main() {
	ctx := context.Background()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		fmt.Printf("Failed to load AWS config: %v\n", err)
		return
	}

	client := s3.NewFromConfig(cfg)

	fmt.Println("S3 Blocks Backfill Example")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("DISCOVERING S3 STRUCTURE")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// List checkpoints
	checkpoints, err := listS3(ctx, client, BlocksPrefix+"/")
	if err != nil {
		fmt.Printf("Error listing checkpoints: %v\n", err)
		return
	}
	fmt.Printf("Checkpoints: %v\n", checkpoints)

	if len(checkpoints) > 0 {
		latest := checkpoints[len(checkpoints)-1]
		dates, _ := listS3(ctx, client, fmt.Sprintf("%s/%s/", BlocksPrefix, latest))
		if len(dates) > 5 {
			fmt.Printf("Dates in checkpoint %s: %v ...\n", latest, dates[:5])
		} else {
			fmt.Printf("Dates in checkpoint %s: %v\n", latest, dates)
		}
	}

	// Example: find and stream a block (commented to avoid S3 charges)
	//
	// br, _ := findBlockFile(ctx, client, 830_000_000)
	// if br != nil {
	//     fmt.Printf("Found in %s\n", br.S3Key)
	//     blocks := streamBlocks(ctx, client, br)
	//     for block := range blocks {
	//         if block.BlockNumber == 830_000_000 {
	//             data, _ := json.MarshalIndent(block, "", "  ")
	//             fmt.Println(string(data))
	//             break
	//         }
	//     }
	// }
}

// listS3 lists objects under an S3 prefix
func listS3(ctx context.Context, client *s3.Client, prefix string) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:       aws.String(S3Bucket),
		Prefix:       aws.String(prefix),
		Delimiter:    aws.String("/"),
		RequestPayer: "requester",
	}

	result, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	var items []string

	// Directories
	for _, p := range result.CommonPrefixes {
		name := strings.TrimPrefix(*p.Prefix, prefix)
		name = strings.TrimSuffix(name, "/")
		if name != "" {
			items = append(items, name)
		}
	}

	// Files
	for _, obj := range result.Contents {
		name := strings.TrimPrefix(*obj.Key, prefix)
		if name != "" {
			items = append(items, name)
		}
	}

	sort.Strings(items)
	return items, nil
}

// findBlockFile finds which S3 file contains a specific block number
func findBlockFile(ctx context.Context, client *s3.Client, targetBlock int64) (*BlockRange, error) {
	checkpoints, err := listS3(ctx, client, BlocksPrefix+"/")
	if err != nil || len(checkpoints) == 0 {
		return nil, err
	}

	checkpoint := checkpoints[len(checkpoints)-1]
	dates, err := listS3(ctx, client, fmt.Sprintf("%s/%s/", BlocksPrefix, checkpoint))
	if err != nil {
		return nil, err
	}

	for _, date := range dates {
		files, err := listS3(ctx, client, fmt.Sprintf("%s/%s/%s/", BlocksPrefix, checkpoint, date))
		if err != nil {
			continue
		}

		for _, file := range files {
			key := fmt.Sprintf("%s/%s/%s/%s", BlocksPrefix, checkpoint, date, file)
			br := ParseBlockRange(key)
			if br != nil && br.StartBlock <= targetBlock && targetBlock <= br.EndBlock {
				return br, nil
			}
		}
	}

	return nil, nil
}

// streamBlocks streams blocks from S3. Files are 3-7 GB - streams line-by-line.
func streamBlocks(ctx context.Context, client *s3.Client, blockRange *BlockRange) <-chan Block {
	ch := make(chan Block)

	go func() {
		defer close(ch)

		input := &s3.GetObjectInput{
			Bucket:       aws.String(S3Bucket),
			Key:          aws.String(blockRange.S3Key),
			RequestPayer: "requester",
		}

		result, err := client.GetObject(ctx, input)
		if err != nil {
			fmt.Printf("Error getting object: %v\n", err)
			return
		}
		defer result.Body.Close()

		scanner := bufio.NewScanner(result.Body)
		// Increase buffer for large lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 10*1024*1024)

		lineNumber := int64(0)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				lineNumber++
				continue
			}

			ch <- Block{
				BlockNumber: blockRange.StartBlock + lineNumber,
				Data:        data,
			}
			lineNumber++
		}
	}()

	return ch
}
