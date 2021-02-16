package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/rpc/client"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This package fetches and parses events from Tendermint. Batch handling should be done prior to calling this program.

const (
	churnIntervalBlocks    = 31400
	maxBatchSizePerRequest = 20
	defaultNodeIP          = "138.68.125.107" // random thorchain node ip
	homeURL                = "http://localhost:4242"
)

// Client for stuff
type Client struct {
	statusClient  client.Client
	historyClient client.Client
	blockDB       *mongo.Collection
	cursorHeight  int64
}

type block struct {
	Time    time.Time
	Hash    []byte
	Results *coretypes.ResultBlockResults
}

// Event is just a cleaner version of Tendermint Event.
type Event struct {
	Type       string
	Attributes map[string]string
}

// BlockInfo contains multiple event types and the height of a single block
type BlockInfo struct {
	blockHeight      int64            `bson:"_block_height"`
	bondReward       int64            `bson:"bond_reward"`
	isChurnEvent     bool             `bson:"churn_event"`
	churnInformation ChurnInformation `bson:"churn_information"`
}

// ChurnInformation lol
type ChurnInformation struct {
	churnedIn  []string
	churnedOut []string
}

func newClient(endpoint *url.URL, timeout time.Duration) (*Client, error) {
	path := endpoint.Path
	endpoint.Path = ""
	remote := endpoint.String()

	tendermintClient, err := rpchttp.NewWithClient(remote, path, &http.Client{Timeout: timeout})

	if err != nil {
		return nil, fmt.Errorf("Error at Tendermint RPC client instantiation: %w", err)
	}

	dbClient, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017/"))
	if err != nil {
		log.Fatal(err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	err = dbClient.Connect(ctx)

	if err != nil {
		return nil, fmt.Errorf("Error at MongoDB client instantiation: %w", err)
	}

	blockCollection := dbClient.Database("thorchain").Collection("blocks")

	return &Client{
		historyClient: tendermintClient,
		statusClient:  tendermintClient,
		blockDB:       blockCollection,
		cursorHeight:  1,
	}, nil
}

func (client *Client) initDBHandler() {
	blockCount, err := client.blockDB.CountDocuments(context.Background(), bson.D{{}})
	if err != nil {
		fmt.Printf("Error retrieving MongoDB document count: %v\n", err)
	}
	_, err = client.blockDB.Find(context.Background(), bson.D{primitive.E{Key: "_block_height", Value: blockCount}})

	if err != nil {
		fmt.Printf("Database corrupted! %d blocks stored but last block not existent: %v", blockCount, err)
		panic("Drop blocks collection and rebuild")

	} else {
		client.cursorHeight = blockCount + 1
		fmt.Printf("Continue parsing from %d\n", blockCount)

	}
}

func (client *Client) insertBatch(batch []BlockInfo) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	for _, blockInfo := range batch {
		_, err := client.blockDB.InsertOne(ctx, blockInfo)
		if err != nil {
			panic("Error inserting block to collection!")
		}
	}
}

func (client *Client) fetchBlockBatch(from, to int64) ([]BlockInfo, error) {
	var blockBatch = make([]BlockInfo, to-from+1)

	info, err := client.historyClient.BlockchainInfo(from, to)
	if err != nil {
		fmt.Printf("Error Fetching Results: %s", err)
		return nil, fmt.Errorf("Can not fetch blockchaininfo: %s", err)

	}

	blocks, err := client.fetchResults(from, to)
	if err != nil {
		fmt.Printf("Error Fetching Blocks: %s", err)
		return nil, fmt.Errorf("Can not fetch results: %s", err)
	}

	blocksLen := len(info.BlockMetas)
	for i := 0; i < blocksLen; i++ {
		meta := info.BlockMetas[(blocksLen-1)-i]
		block := blocks[i]
		if block == nil {
			fmt.Printf("Empty Block")
			continue
		}
		events, err := client.parseBlock(meta, block)
		if err != nil {
			fmt.Printf("Error while executing block - skipping")
			continue
		}

		blockBatch[i] = events

	}

	return blockBatch, nil
}

func (client *Client) fetchResults(from, to int64) ([]*coretypes.ResultBlockResults, error) {
	blocks := make([]*coretypes.ResultBlockResults, 0, to-from+1)
	if to == from {
		block, err := client.historyClient.BlockResults(&from)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch block results for height %d", from)
		}
		blocks = append(blocks, block)
	} else {
		for i := from; i <= to; i++ {
			block, err := client.historyClient.BlockResults(&i)
			if err != nil {
				return nil, errors.Wrapf(err, "could not prepare request block results of height %d", i)
			}
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func (client *Client) parseBlock(meta *types.BlockMeta, block *coretypes.ResultBlockResults) (BlockInfo, error) {
	var blockInfo BlockInfo
	blockInfo.blockHeight = meta.Header.Height
	blockInfo.isChurnEvent = false

	/*	Currently not used but might as well leave it in for completeness
			for _, tx := range block.TxsResults {
			event := convertEvents(tx.Events)
			fmt.Printf("%s ", event)
			blockEvents.TransactionEvents = append(blockEvents.TransactionEvents, event)
		}
		beginEvents := convertEvents(block.BeginBlockEvents)
		blockEvents.BeginBlockEvents = append(blockEvents.BeginBlockEvents, beginEvents)
	*/

	endEvents := convertEvents(block.EndBlockEvents)
	for i := 0; i < len(endEvents); i++ {

		if endEvents[i].Type == "rewards" {
			blockInfo.bondReward, _ = strconv.ParseInt(endEvents[i].Attributes["bond_reward"], 10, 64)
		} else if endEvents[i].Type == "UpdateNodeAccountStatus" {
			blockInfo.isChurnEvent = true
			if endEvents[i].Attributes["Current:"] == "active" && endEvents[i].Attributes["Former:"] == "ready" {
				blockInfo.churnInformation.churnedIn = append(blockInfo.churnInformation.churnedIn, endEvents[i].Attributes["Address"])
			} else if endEvents[i].Attributes["Current:"] == "standby" && endEvents[i].Attributes["Former:"] == "active" {
				blockInfo.churnInformation.churnedOut = append(blockInfo.churnInformation.churnedOut, endEvents[i].Attributes["Address"])
			} else {
				fmt.Printf("Something happend, which should not happen at all!")
			}
		}
	}

	return blockInfo, nil
}

func (client *Client) getCurrentBlockHeight() (int64, error) {
	status, err := client.statusClient.Status()

	if err != nil {
		fmt.Printf("Error getting status %d\n", err)
		return 0, err
	}

	return status.SyncInfo.LatestBlockHeight, nil
}

func convertEvents(tes []abcitypes.Event) []Event {
	events := make([]Event, len(tes))
	for i, te := range tes {
		events[i].FromTendermintEvent(te)
	}

	return events
}

// FromTendermintEvent converts Tendermint native Event structure to Event.
// https://gitlab.com/thorchain/midgard/-/blob/master/pkg/clients/thorchain/event.go
func (e *Event) FromTendermintEvent(te abcitypes.Event) {
	e.Type = te.Type
	e.Attributes = make(map[string]string, len(te.Attributes))
	for _, kv := range te.Attributes {
		e.Attributes[string(kv.Key)] = string(kv.Value)
	}
}

func main() {
	var nodeURL *url.URL
	var sb strings.Builder
	sb.WriteString("http://")
	sb.WriteString(defaultNodeIP)
	sb.WriteString(":27147/websocket")
	nodeURL, _ = url.Parse(sb.String())
	timeout := time.Duration(100 * time.Second)
	client, _ := newClient(nodeURL, timeout)

	client.initDBHandler()

	var dbSynced sync.WaitGroup

	for {
		currentBlockHeight, _ := client.getCurrentBlockHeight()
		fmt.Printf("%d / %d  Progress: %d\n", client.cursorHeight, currentBlockHeight, (client.cursorHeight * 100 / currentBlockHeight))

		batchSize := currentBlockHeight - client.cursorHeight

		if batchSize > maxBatchSizePerRequest {
			batchSize = maxBatchSizePerRequest
		} else if batchSize <= 0 {
			fmt.Printf("Kowalski, we are live\n")
			time.Sleep(time.Second * 2)
			continue
		}

		blockBatch, err := client.fetchBlockBatch(client.cursorHeight, client.cursorHeight+batchSize-1)
		if err != nil {
			fmt.Printf("Error parsing events: %s", err.Error())
			return
		}

		dbSynced.Wait()

		go func() {
			dbSynced.Add(1)
			client.insertBatch(blockBatch)
			dbSynced.Done()
		}()

		client.cursorHeight += batchSize
	}

}
