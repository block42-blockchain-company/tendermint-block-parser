# Tendermint Block Parser
A Tendermint client fetches blocks from a specified node and stores certain events occurences in for each block in MongoDB.
In the current case these are the Thorchain events ``bond_rewards`` and ``UpdateNodeStatus``. Never-the-less, it works for all CosmosSDK projects, since it retrieves [ABCI](https://docs.cosmos.network/master/intro/sdk-design.html) data, which is not business logic specific.

This daemon runs until it catches up with current block height and then alternates between sleeping and fetching.
Make sure to have a MongoDB instance set up at ``localhost:27017``. 

This project is part of the [Thorchain Telegram Bot](https://github.com/block42-blockchain-company/thornode-telegram-bot).
The Tendermint client used is still on version 0.33.4, which uses the amino protocol (with version 0.34.4  protobuf was introduced).

To build it run ``go build ./tmClient.go``.

To run it type ``./tmClient``

