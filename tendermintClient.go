package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/block42-blockchain-company/tendermint-block-parser/parser"
)

const (
	maxBatchSizePerRequest = 20
	defaultNodeIP          = "138.68.125.107" // random thorchain node ip
	version                = 1.0
)

func main() {
	fmt.Printf("Starting Tendermint Client v%f\n", version)

	var nodeURL *url.URL
	var sb strings.Builder
	sb.WriteString("http://")
	sb.WriteString(defaultNodeIP)
	port := ":27147/"
	if os.Getenv("TESTNET") == "True" {
		fmt.Println("Use Tendermint Testnet Port")
		port = ":26657/"
	}
	sb.WriteString(port)
	sb.WriteString("websocket")
	nodeURL, _ = url.Parse(sb.String())

	timeout := 100 * time.Second
	thorchainParser, _ := parser.NewParser(nodeURL, timeout)

	thorchainParser.Setup()
	var dbSynced sync.WaitGroup

	for {
		currentBlockHeight, _ := thorchainParser.GetCurrentBlockHeight()
		batchSize := currentBlockHeight - thorchainParser.CursorHeight

		if batchSize > maxBatchSizePerRequest {
			batchSize = maxBatchSizePerRequest
		} else if batchSize <= 0 {
			fmt.Printf("Kowalski, we are live\n")
			time.Sleep(time.Second * 2)
			continue
		}

		blockBatch, err := thorchainParser.FetchBlockBatch(thorchainParser.CursorHeight, thorchainParser.CursorHeight+batchSize-1)
		if err != nil {
			fmt.Printf("Error parsing events: %s", err.Error())
			panic("Could not fetch blocks. Please see logs")
		}

		dbSynced.Wait()
		go func() {
			dbSynced.Add(1)
			thorchainParser.AddBlocksToChurnCycle(blockBatch)
			dbSynced.Done()
		}()

		thorchainParser.CursorHeight += batchSize
	}
}
