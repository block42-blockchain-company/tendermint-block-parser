# Tendermint Block Parser
A Tendermint client fetches blocks from a specified node and stores certain events occurrences in for each block in MongoDB.
In the current case these are the Thorchain events ``bond_rewards`` and ``UpdateNodeStatus``. Never-the-less, it works for all CosmosSDK projects, since it retrieves [ABCI](https://docs.cosmos.network/master/intro/sdk-design.html) data, which is not business logic specific.

This daemon runs until it catches up with current block height and then alternates between sleeping and fetching.
Per default, it will connect to a MongoDB instance behind ``mongodb://thornode_bot_mongodb:27017``. 
If ``TM_CLIENT_DEV`` is passed, it will connect to ``mongodb://localhost:27017``.

This project is part of the [Thorchain Telegram Bot](https://github.com/block42-blockchain-company/thornode-telegram-bot).
It is provided as a docker image on [DockerHub](https://hub.docker.com/repository/docker/block42blockchaincompany/thorchain-parser/general).
The Tendermint client used is still on version 0.33.4, which uses the amino protocol (with version 0.34.4  protobuf was introduced).

To build it run ``go build ./tendermintClient.go``.

To run it type ``./tendermintClient``

