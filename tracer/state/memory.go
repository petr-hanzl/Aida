package state

import (
	"fmt"

	geth "github.com/Fantom-foundation/substate-cli/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/substate"
)

func MakeGethInMemoryStateDB(variant string) (StateDB, error) {
	if variant != "" {
		return nil, fmt.Errorf("unkown variant: %v", variant)
	}
	return &gethInMemoryStateDB{}, nil
}

type gethInMemoryStateDB struct {
	gethStateDB
}

func (s *gethInMemoryStateDB) BeginBlockApply(root_hash common.Hash) error {
	return nil
}

func (s *gethInMemoryStateDB) Close() error {
	// Nothing to do.
	return nil
}

func (s *gethInMemoryStateDB) PrepareSubstate(substate *substate.SubstateAlloc) {
	s.db = geth.MakeInMemoryStateDB(substate)
}
