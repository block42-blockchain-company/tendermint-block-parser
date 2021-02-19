package parser

import (
	"context"
	"fmt"
	"github.com/block42-blockchain-company/tmClient/types"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/rpc/client"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	tmTypes "github.com/tendermint/tendermint/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)


// Parser for stuff
type Parser struct {
	statusClient  client.Client     // Current block height
	historyClient client.Client		// Fetch blocks
	blockDB       *mongo.Collection
	churnDB       *mongo.Collection
	configDB 	  *mongo.Collection
	CursorHeight  int64
	currentChurn  types.ChurnCycle // ongoing Churn Cycle
}


func NewParser(endpoint *url.URL, timeout time.Duration) (*Parser, error) {
	path := endpoint.Path
	endpoint.Path = ""
	remote := endpoint.String()

	tendermintClient, err := rpchttp.NewWithClient(remote, path, &http.Client{Timeout: timeout})

	if err != nil {
		return nil, fmt.Errorf("Error at Tendermint RPC parser instantiation: %w", err)
	}

	dbClient, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017/"))
	if err != nil {
		log.Fatal(err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = dbClient.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error at MongoDB parser instantiation: %w", err)
	}

	churnCycleCollection := dbClient.Database("thorchain").Collection("churns")
	configCollection := dbClient.Database("thorchain").Collection("config")

	defaultChurn := types.ChurnCycle{
		ChurnNumber: 0,
		BlockHeightStart: 1,
		BlockHeightEnd: 1,
		ValidatorSet: nil,
		TotalAddedRewards: 0,
	}

	return &Parser{
		CursorHeight:  1,
		currentChurn:  defaultChurn,
		historyClient: tendermintClient,
		statusClient:  tendermintClient,
		churnDB:       churnCycleCollection,
		configDB: 	   configCollection,
	}, nil
}


func (parser *Parser) Setup() {
	currentChurnCursor := parser.configDB.FindOne(context.Background(), bson.D{})

	var currentChurn types.ChurnCycle
	if err := currentChurnCursor.Decode(&currentChurn); err != nil {
		if _, err := parser.configDB.InsertOne(context.Background(), parser.currentChurn); err != nil {
			fmt.Printf("Error: %s\n",err)

			panic("Could not store initial churn cycle. Aborting ...")
		}
		fmt.Printf("No configuration found. Start parsing from height %d.\n", parser.currentChurn.ChurnNumber)
		return
	}


	fmt.Printf("Retrieved configuration. Starting sanity checks... \n")

	churnCycleCount, _ := parser.churnDB.CountDocuments(context.Background(), bson.D{{}})
	if churnCycleCount != (currentChurn.ChurnNumber) {
		fmt.Printf("Churn cycle count: %d does not match current churn cycle number: %d\n",
			churnCycleCount, currentChurn.ChurnNumber)
		panic("Sanity check failed.\n")
	}

	parser.currentChurn = currentChurn
	parser.CursorHeight = currentChurn.BlockHeightEnd + 1
	fmt.Printf("Setup parser finished. Start parsing blocks from %d in churn cycle %d\n",
			   parser.CursorHeight, churnCycleCount)
}


func (parser *Parser) PersistState() {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := parser.configDB.ReplaceOne(ctx, bson.M{"_churn_number": parser.currentChurn.ChurnNumber}, parser.currentChurn)
	if err != nil {
		fmt.Printf("Error %s\n", err)
		panic("Parser state could not be persisted! Drop DB and restart.")
	}
}


func (parser *Parser) AddBlocksToChurnCycle(batch []types.BlockInfo) {
	for _, blockInfo := range batch {

		// Same Churn Cycle
		if !blockInfo.IsChurnEvent {
			parser.currentChurn.TotalAddedRewards += blockInfo.BondReward
			parser.currentChurn.BlockHeightEnd = blockInfo.BlockHeight
			continue
		}

		// New Churn Cycle
		newValidatorSet := parser.currentChurn.ValidatorSet

		// Churn out validators
		for _, validator := range blockInfo.ChurnInformation.ChurnedOut {
			if err := parser.currentChurn.RemoveValidatorFromSet(validator); err != nil {
				panic("Fatal: Validator churn out failure. Not in set!\n")
			}
		}

		// Reset validators slash points
		for _, validator := range newValidatorSet {
			validator.SlashPoints = 0
		}

		// Churn in validators
		for _, validator := range blockInfo.ChurnInformation.ChurnedIn {
			newValidator := &types.ChurnValidator{
				Address: validator,
				SlashPoints: 0,
			}
			newValidatorSet = append(parser.currentChurn.ValidatorSet, *newValidator)
		}


		if _, err := parser.churnDB.InsertOne(context.Background(), parser.currentChurn); err != nil {
			panic("Could not store churn cycle. Aborting ...")
		}

		parser.currentChurn = types.ChurnCycle{
			ChurnNumber: parser.currentChurn.ChurnNumber + 1,
			BlockHeightStart: blockInfo.BlockHeight,
			BlockHeightEnd: blockInfo.BlockHeight,
			TotalAddedRewards: blockInfo.BondReward,
			ValidatorSet: newValidatorSet,
		}
		parser.PersistState()
		fmt.Printf("Successfully cycled churn. Current churn number %d\n", parser.currentChurn.ChurnNumber)
	}

	parser.PersistState()
}

func (parser *Parser) FetchBlockBatch(from, to int64) ([]types.BlockInfo, error) {
	var blockBatch = make([]types.BlockInfo, to-from+1)

	info, err := parser.historyClient.BlockchainInfo(from, to)
	if err != nil {
		fmt.Printf("Error Fetching Results: %s", err)
		return nil, fmt.Errorf("can not fetch blockchaininfo: %s", err)
	}

	blocks, err := parser.fetchResults(from, to)
	if err != nil {
		fmt.Printf("Error Fetching Blocks: %s", err)
		return nil, fmt.Errorf("can not fetch results: %s", err)
	}

	blocksLen := len(info.BlockMetas)
	for i := 0; i < blocksLen; i++ {
		meta := info.BlockMetas[(blocksLen-1)-i]
		block := blocks[i]
		if block == nil {
			fmt.Printf("Empty Block")
			continue
		}
		blockInfo, err := parser.parseBlock(meta, block)

		if err != nil {
			fmt.Printf("Error while executing block - skipping")
			continue
		}

		blockBatch[i] = blockInfo
	}
	return blockBatch, nil
}

func (parser *Parser) fetchResults(from, to int64) ([]*coretypes.ResultBlockResults, error) {
	blocks := make([]*coretypes.ResultBlockResults, 0, to-from+1)
	if to == from {
		block, err := parser.historyClient.BlockResults(&from)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch block results for height %d", from)
		}
		blocks = append(blocks, block)
	} else {
		for i := from; i <= to; i++ {
			block, err := parser.historyClient.BlockResults(&i)
			if err != nil {
				return nil, errors.Wrapf(err, "could not prepare request block results of height %d", i)
			}
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func (parser *Parser) parseBlock(meta *tmTypes.BlockMeta, block *coretypes.ResultBlockResults) (types.BlockInfo, error) {
	var blockInfo types.BlockInfo
	blockInfo.BlockHeight = meta.Header.Height
	blockInfo.IsChurnEvent = false

	/*
	Begin block events
	TX events
	Currently not used but might as well leave it in for completeness

		for _, tx := range block.TxsResults {
			event := convertEvents(tx.Events)
			fmt.Printf("%s ", event)
			blockEvents.TransactionEvents = append(blockEvents.TransactionEvents, event)
		}
		beginEvents := convertEvents(block.BeginBlockEvents)
		blockEvents.BeginBlockEvents = append(blockEvents.BeginBlockEvents, beginEvents)
	*/

	endEvents := types.ConvertEvents(block.EndBlockEvents)
	for i := 0; i < len(endEvents); i++ {

		if endEvents[i].Type == "rewards" {
			blockInfo.BondReward, _ = strconv.ParseInt(endEvents[i].Attributes["bond_reward"], 10, 64)
		} else if endEvents[i].Type == "UpdateNodeAccountStatus" {
			blockInfo.IsChurnEvent = true
			if endEvents[i].Attributes["Current:"] == "active" && endEvents[i].Attributes["Former:"] == "ready" {
				blockInfo.ChurnInformation.ChurnedIn = append(blockInfo.ChurnInformation.ChurnedIn, endEvents[i].Attributes["Address"])
			} else if endEvents[i].Attributes["Current:"] == "standby" && endEvents[i].Attributes["Former:"] == "active" {
				blockInfo.ChurnInformation.ChurnedOut = append(blockInfo.ChurnInformation.ChurnedOut, endEvents[i].Attributes["Address"])
			} else {
				fmt.Printf("Something happend, which should not happen at all!")
			}
		}
	}

	return blockInfo, nil
}

func (parser *Parser) GetCurrentBlockHeight() (int64, error) {
	status, err := parser.statusClient.Status()

	if err != nil {
		fmt.Printf("Error getting status %d\n", err)
		return 0, err
	}

	return status.SyncInfo.LatestBlockHeight, nil
}