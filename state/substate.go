// Copyright 2024 Fantom Foundation
// This file is part of Aida Testing Infrastructure for Sonic
//
// Aida is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Aida is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Aida. If not, see <http://www.gnu.org/licenses/>.

package state

// A legacy code for substate-cli
import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/Fantom-foundation/Aida/txcontext"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
)

// offTheChainDB is state.cachingDB clone without disk caches
type offTheChainDB struct {
	db *trie.Database
}

// OpenTrie opens the main account trie at a specific root hash.
func (db *offTheChainDB) OpenTrie(root common.Hash) (state.Trie, error) {
	tr, err := trie.NewSecure(root, db.db)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

// OpenStorageTrie opens the storage trie of an account.
func (db *offTheChainDB) OpenStorageTrie(addrHash, root common.Hash) (state.Trie, error) {
	tr, err := trie.NewSecure(root, db.db)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

// CopyTrie returns an independent copy of the given trie.
func (db *offTheChainDB) CopyTrie(t state.Trie) state.Trie {
	switch t := t.(type) {
	case *trie.SecureTrie:
		return t.Copy()
	default:
		panic(fmt.Errorf("unknown trie type %T", t))
	}
}

// ContractCode retrieves a particular contract's code.
func (db *offTheChainDB) ContractCode(addrHash, codeHash common.Hash) ([]byte, error) {
	code := rawdb.ReadCode(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		return code, nil
	}
	return nil, errors.New("not found")
}

// ContractCodeWithPrefix retrieves a particular contract's code. If the
// code can't be found in the cache, then check the existence with **new**
// db scheme.
func (db *offTheChainDB) ContractCodeWithPrefix(addrHash, codeHash common.Hash) ([]byte, error) {
	code := rawdb.ReadCodeWithPrefix(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		return code, nil
	}
	return nil, errors.New("not found")
}

// ContractCodeSize retrieves a particular contracts code's size.
func (db *offTheChainDB) ContractCodeSize(addrHash, codeHash common.Hash) (int, error) {
	code, err := db.ContractCode(addrHash, codeHash)
	return len(code), err
}

// TrieDB retrieves any intermediate trie-node caching layer.
func (db *offTheChainDB) TrieDB() *trie.Database {
	return db.db
}

// NewOffTheChainStateDB returns an empty in-memory *state.StateDB without disk caches
func NewOffTheChainStateDB() *state.StateDB {
	// backend in-memory key-value database
	kvdb := rawdb.NewMemoryDatabase()

	// zeroed trie.Config to disable Cache, Journal, Preimages, ...
	zerodConfig := &trie.Config{}
	tdb := trie.NewDatabaseWithConfig(kvdb, zerodConfig)

	sdb := &offTheChainDB{
		db: tdb,
	}

	statedb, err := state.New(common.Hash{}, sdb, nil)
	if err != nil {
		panic(fmt.Errorf("error calling state.New() in NewOffTheChainDB(): %v", err))
	}
	return statedb
}

type code_key struct {
	addr common.Address
	code string
}

// todo remove globals in upcoming PR!
var code_hashes_lock = sync.Mutex{}
var code_hashes = map[code_key]common.Hash{}

func getHash(addr common.Address, code []byte) common.Hash {
	key := code_key{addr, string(code)}
	code_hashes_lock.Lock()
	res, exists := code_hashes[key]
	code_hashes_lock.Unlock()
	if exists {
		return res
	}
	res = crypto.Keccak256Hash(code)
	code_hashes_lock.Lock()
	code_hashes[key] = res
	code_hashes_lock.Unlock()
	return res
}

// MakeOffTheChainStateDB returns an in-memory *state.StateDB initialized with ws
func MakeOffTheChainStateDB(alloc txcontext.WorldState, block uint64, chainConduit *ChainConduit) (StateDB, error) {
	statedb := NewOffTheChainStateDB()
	alloc.ForEachAccount(func(addr common.Address, acc txcontext.Account) {
		code := acc.GetCode()
		statedb.SetPrehashedCode(addr, getHash(addr, code), code)
		statedb.SetNonce(addr, acc.GetNonce())
		statedb.SetBalance(addr, acc.GetBalance())
		// DON'T USE SetStorage because it makes REVERT and dirtyStorage unavailble
		acc.ForEachStorage(func(keyHash common.Hash, valueHash common.Hash) {
			statedb.SetState(addr, keyHash, valueHash)
		})
	})

	// Commit and re-open to start with a clean state.
	_, err := statedb.Commit(false)
	if err != nil {
		return nil, fmt.Errorf("cannot commit offTheChainDb; %v", err)
	}

	blk := new(big.Int).SetUint64(block)
	return &gethStateDB{db: statedb, block: blk, chainConduit: chainConduit}, nil
}

func ReleaseCache() {
	code_hashes_lock.Lock()
	code_hashes = map[code_key]common.Hash{}
	code_hashes_lock.Unlock()
}
