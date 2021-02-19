package types

// BlockInfo contains multiple event types and the height of a single block
type BlockInfo struct {
	BlockHeight      int64            `bson:"_block_height"`
	BondReward       int64            `bson:"bond_reward"`
	IsChurnEvent     bool             `bson:"churn_event"`
	ChurnInformation ChurnInformation `bson:"churn_information"`
}

// ChurnInformation new and old validators
type ChurnInformation struct {
	ChurnedIn  []string		`bson:"churned_in"`
	ChurnedOut []string		`bson:"churned_out"`
}
