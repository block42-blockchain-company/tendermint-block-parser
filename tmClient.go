package main

import (
	"fmt"
	"github.com/block42-blockchain-company/tmClient/parser"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	maxBatchSizePerRequest = 20
	defaultNodeIP          = "138.68.125.107" // random thorchain node ip
)


func main() {
	var nodeURL *url.URL
	var sb strings.Builder
	sb.WriteString("http://")
	sb.WriteString(defaultNodeIP) // TODO Inject nodeIP, otherwise it's ddos when 200 bots are active
	sb.WriteString(":27147/websocket")
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
