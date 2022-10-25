// Package snapshot implements database interfaces for the world state manager.
package snapshot

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/substate"
	"sync"
)

// SubstateIterator defines an interface of substate iterator we use to collect addresses.
type SubstateIterator interface {
	Next() bool
	Value() *substate.Transaction
	Release()
}

// ZeroAddressBytes represents the content of an empty address.
var ZeroAddressBytes = common.Address{}.Bytes()

// CollectAccounts collects accounts from the substate database iterator and sends them for processing.
func CollectAccounts(ctx context.Context, in SubstateIterator, toBlock uint64, workers int) <-chan common.Address {
	out := make(chan common.Address, workers*10)
	go collectSubStateAccounts(ctx, in, toBlock, out, workers)
	return out
}

// WriteAccountAddresses writes account addresses received from an input queue into world state snapshot database.
func WriteAccountAddresses(ctx context.Context, in <-chan common.Address, db *StateDB) error {
	var hashing = crypto.NewKeccakState()

	ctxDone := ctx.Done()
	for {
		select {
		case <-ctxDone:
			return ctx.Err()
		case adr, open := <-in:
			if !open {
				return nil
			}

			// calculate the hash of the account
			err := db.PutHashToAccountAddress(crypto.HashData(hashing, adr.Bytes()), adr)
			if err != nil {
				return err
			}
		}
	}
}

// FilterUniqueAccounts extracts accounts from input queue and sends only unique occurrences to the output.
// The filter will close output channel once done processing incoming accounts.
func FilterUniqueAccounts(ctx context.Context, in <-chan common.Address, out chan<- common.Address) {
	defer close(out)

	visited := make(map[common.Address]bool, 20_000_000)
	ctxDone := ctx.Done()
	for {
		select {
		case <-ctxDone:
			return
		case adr, open := <-in:
			if !open {
				return
			}

			// is this a unique address?
			_, found := visited[adr]
			if found {
				continue
			}

			select {
			case <-ctxDone:
				return
			case out <- adr:
				visited[adr] = true
			}
		}
	}
}

// collectSubStateAccounts iterates SubState database transactions and sends them for processing to a worker channel.
// The iteration walker will close output channel once all the internal workers are done collecting addresses.
func collectSubStateAccounts(ctx context.Context, in SubstateIterator, toBlock uint64, out chan<- common.Address, workers int) {
	defer close(out)

	// prepare structures for account collectors
	work := make(chan *substate.Transaction, workers)
	var wg sync.WaitGroup

	// start account collector workers
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go extractSubStateAccounts(ctx, work, out, &wg)
	}

	// iterate SubStates to get transactions
	ctxDone := ctx.Done()
	for in.Next() {
		tx := in.Value()
		if toBlock > 0 && tx.Block > toBlock {
			break
		}

		select {
		case <-ctxDone:
			break
		case work <- tx:
		}
	}

	// signal workers we are done iterating and wait for their termination
	close(work)
	wg.Wait()
}

// extractSubStateAccounts worker executes account collector job on substate records received via input queue.
// Found account addresses are sent to an output queue for processing.
// We do not care about sending unique address from the worker since it's expected to run in parallel,
// the filtering should be done later down the queue line.
func extractSubStateAccounts(ctx context.Context, in <-chan *substate.Transaction, out chan<- common.Address, wg *sync.WaitGroup) {
	defer wg.Done()

	ctxDone := ctx.Done()
	for {
		select {
		case <-ctxDone:
			return
		case tx, open := <-in:
			if !open {
				return
			}

			// non-zero env coinbase
			if tx.Substate.Env != nil && !bytes.Equal(tx.Substate.Env.Coinbase.Bytes(), ZeroAddressBytes) {
				out <- tx.Substate.Env.Coinbase
			}

			// message sender and recipient
			if tx.Substate.Message != nil {
				out <- tx.Substate.Message.From

				if tx.Substate.Message.To != nil {
					out <- *tx.Substate.Message.To
				}
			}

			// input alloc
			if tx.Substate.InputAlloc != nil {
				for adr := range tx.Substate.InputAlloc {
					out <- adr
				}
			}

			// output alloc
			if tx.Substate.OutputAlloc != nil {
				for adr := range tx.Substate.OutputAlloc {
					out <- adr
				}
			}

			// non-zero result contract address
			if tx.Substate.Result != nil && !bytes.Equal(tx.Substate.Result.ContractAddress.Bytes(), ZeroAddressBytes) {
				out <- tx.Substate.Result.ContractAddress
			}
		}
	}
}