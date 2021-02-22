package types

import "github.com/pkg/errors"

// ChurnCycle is the condensed information of all blocks in that specific cycle including absolute state information
type ChurnCycle struct {
	ChurnNumber		  int64				`bson:"_churn_number"`
	BlockHeightStart  int64				`bson:"block_height_start"`
	BlockHeightEnd    int64				`bson:"block_height_end"`
	ValidatorSet      []ChurnValidator	`bson:"validator_set"`
	TotalAddedRewards int64				`bson:"total_added_rewards"`
}

type ChurnValidator struct {
	Address     string
	SlashPoints int64
}

func (cc *ChurnCycle) RemoveValidatorFromSet(validatorAddress string) error {
	for i, validator := range cc.ValidatorSet {
		if validator.Address == validatorAddress {
			cc.ValidatorSet = append(cc.ValidatorSet [:i], cc.ValidatorSet [i+1:]...)
			return nil
		}
	}
	return errors.New("Validator not in current churn cycle")
}