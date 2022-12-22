package trace

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Fantom-foundation/Aida/world-state/db/snapshot"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/substate"
	"github.com/syndtr/goleveldb/leveldb"
)

// generateUpdateSet generates an update set for a block range.
func generateUpdateSet(first uint64, last uint64, cfg *TraceConfig) substate.SubstateAlloc {
	var deletedAccountDB *substate.DestroyedAccountDB
	stateIter := substate.NewSubstateIterator(first, cfg.workers)
	defer stateIter.Release()
	if cfg.hasDeletedAccounts {
		deletedAccountDB = substate.OpenDestroyedAccountDBReadOnly(cfg.deletedAccountDir)
		defer deletedAccountDB.Close()
	}

	update := make(substate.SubstateAlloc)
	for stateIter.Next() {
		tx := stateIter.Value()
		// exceeded block range?
		if tx.Block > last {
			break
		}

		// if this transaction has suicided accounts, clear their states.
		if cfg.hasDeletedAccounts {
			destroyed, resurrected, err := deletedAccountDB.GetDestroyedAccounts(tx.Block, tx.Transaction)

			if !(err == nil || errors.Is(err, leveldb.ErrNotFound)) {
				log.Fatalf("failed to get deleted account. %v", err)
			}
			// reset storage
			resetStorageFromList(update, destroyed)
			resetStorageFromList(update, resurrected)
		}

		// merge output substate to update
		update.Merge(tx.Substate.OutputAlloc)
	}
	return update
}

// generateWorldStateFromUpdateDB generates an initial world-state
// from pre-computed update-set
func generateWorldStateFromUpdateDB(cfg *TraceConfig, target uint64) (substate.SubstateAlloc, error) {
	ws := make(substate.SubstateAlloc)
	blockPos := uint64(FirstSubstateBlock - 1)
	if target < blockPos {
		return nil, fmt.Errorf("Error: the target block, %v, is earlier than the initial world state block, %v. The world state is not loaded.\n", target, blockPos)
	}
	// load pre-computed update-set from update-set db
	db := substate.OpenUpdateDBReadOnly(cfg.updateDBDir)
	defer db.Close()
	updateIter := substate.NewUpdateSetIterator(db, blockPos, target, cfg.workers)
	for updateIter.Next() {
		blk := updateIter.Value()
		if blk.Block > target {
			break
		}
		blockPos = blk.Block
		// Reset accessed storage locations of suicided accounts prior to updateset block.
		// The known accessed storage locations in the updateset range has already been
		// reset when generating update set database.
		resetStorageFromList(ws, blk.DeletedAccounts)
		ws.Merge(*blk.UpdateSet)
	}
	updateIter.Release()

	// advance from the latest precomputed block to the target block
	update := generateUpdateSet(blockPos+1, target, cfg)
	ws.Merge(update)

	return ws, nil
}

// resetStorageFromList sets storage of addresses in a list to common.Hash{}
func resetStorageFromList(ws substate.SubstateAlloc, addresses []common.Address) {
	for _, addr := range addresses {
		if _, found := ws[addr]; found {
			ws[addr].Storage = make(map[common.Hash]common.Hash)
		}
	}
}

// generateWorldState generates an initial world-state for a block.
func generateWorldState(path string, block uint64, cfg *TraceConfig) (substate.SubstateAlloc, error) {
	worldStateDB, err := snapshot.OpenStateDB(path)
	if err != nil {
		return nil, err
	}
	defer snapshot.MustCloseStateDB(worldStateDB)
	ws, err := worldStateDB.ToSubstateAlloc(context.Background())
	if err != nil {
		return nil, err
	}

	// advance from the first block from substateDB to the target block
	update := generateUpdateSet(FirstSubstateBlock, block, cfg)
	ws.Merge(update)

	return ws, nil
}
