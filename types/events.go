package types

import abcitypes "github.com/tendermint/tendermint/abci/types"

// Event is just a cleaner version of Tendermint Event.
type Event struct {
	Type       string
	Attributes map[string]string
}

func ConvertEvents(tes []abcitypes.Event) []Event {
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