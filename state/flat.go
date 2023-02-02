package state

import (
	"fmt"
	"math/big"

	"github.com/Fantom-foundation/go-opera-fvm/flat"
	"github.com/Fantom-foundation/go-opera-fvm/gossip/evmstore/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/substate"
)

func MakeFlatStateDB(directory, variant string) (s StateDB, err error) {
	var db ethdb.Database

	switch variant {
	case "go-memory":
		db = rawdb.NewMemoryDatabase()
	case "go-ldb":
		const cache_size = 512
		const file_handle = 128
		db, err = rawdb.NewLevelDBDatabase(directory, cache_size, file_handle, "", false)
		if err != nil {
			err = fmt.Errorf("Failed to create a new Level DB. %v", err)
			return
		}
	default:
		err = fmt.Errorf("unkown variant: %v", variant)
		return
	}

	fs := &flatStateDB{
		db:        flat.NewDatabase(db),
		stateRoot: common.Hash{},
	}

	// initialize stateDB
	fs.BeginBlockApply()
	s = fs
	return
}

type flatStateDB struct {
	db        state.Database
	stateRoot common.Hash
	*state.StateDB
}

// BeginBlockApply creates a new statedb from an existing geth database
func (s *flatStateDB) BeginBlockApply() error {
	var err error
	s.StateDB, err = state.New(s.stateRoot, s.db)
	return err
}

func (s *flatStateDB) BeginTransaction(number uint32) {
	// ignored
}

func (s *flatStateDB) EndTransaction() {
	// ignored
}

func (s *flatStateDB) BeginBlock(number uint64) {
	// ignored
}

func (s *flatStateDB) EndBlock() {
	var err error
	//commit at the end of a block
	s.stateRoot, err = s.Commit(true)
	if err != nil {
		panic(fmt.Errorf("StateDB commit failed\n"))
	}
}

func (s *flatStateDB) BeginEpoch(number uint64) {
	// ignored
}

func (s *flatStateDB) EndEpoch() {
	// ignored
}

// PrepareSubstate initiates the state DB for the next transaction.
func (s *flatStateDB) PrepareSubstate(*substate.SubstateAlloc) {
	// ignored
	return
}

func (s *flatStateDB) GetSubstatePostAlloc() substate.SubstateAlloc {
	// ignored
	return substate.SubstateAlloc{}
}

// Close requests the StateDB to flush all its content to secondary storage and shut down.
// After this call no more operations will be allowed on the state.
func (s *flatStateDB) Close() error {
	// Commit data to trie.
	hash, err := s.Commit(true)
	if err != nil {
		return err
	}
	// Close underlying trie caching intermediate results.
	db := s.Database().TrieDB()
	if err := db.Commit(hash, true, nil); err != nil {
		return err
	}
	// Close underlying LevelDB instance.
	err = db.DiskDB().Close()
	if err != nil {
		return err
	}

	return nil
}

func (s *flatStateDB) GetMemoryUsage() *MemoryUsage {
	// not supported yet
	return nil
}

func (s *flatStateDB) StartBulkLoad() BulkLoad {
	return &flatBulkLoad{db: s}
}

// For priming initial state of stateDB
type flatBulkLoad struct {
	db      *flatStateDB
	num_ops int64
}

func (l *flatBulkLoad) CreateAccount(addr common.Address) {
	l.db.CreateAccount(addr)
	l.digest()
}

func (l *flatBulkLoad) SetBalance(addr common.Address, value *big.Int) {
	old := l.db.GetBalance(addr)
	value = value.Sub(value, old)
	l.db.AddBalance(addr, value)
	l.digest()
}

func (l *flatBulkLoad) SetNonce(addr common.Address, nonce uint64) {
	l.db.SetNonce(addr, nonce)
	l.digest()
}

func (l *flatBulkLoad) SetState(addr common.Address, key common.Hash, value common.Hash) {
	l.db.SetState(addr, key, value)
	l.digest()
}

func (l *flatBulkLoad) SetCode(addr common.Address, code []byte) {
	l.db.SetCode(addr, code)
	l.digest()
}

func (l *flatBulkLoad) Close() error {
	l.db.EndBlock()
	_, err := l.db.Commit(false)
	return err
}

func (l *flatBulkLoad) digest() {
	// Call EndBlock every 1M insert operation.
	l.num_ops++
	if l.num_ops%(1000*1000) != 0 {
		return
	}
	l.db.EndBlock()
}