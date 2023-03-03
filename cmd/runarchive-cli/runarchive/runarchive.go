package runarchive

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/Fantom-foundation/Aida/state"
	"github.com/Fantom-foundation/Aida/utils"
	substate "github.com/Fantom-foundation/Substate"
	"github.com/urfave/cli/v2"
)

// RunArchive implements the command evaluating historic transactions on an archive.
func RunArchive(ctx *cli.Context) error {
	var (
		err         error
		start       time.Time
		sec         float64
		lastSec     float64
		txCount     int
		lastTxCount int
	)

	// process general arguments
	cfg, argErr := utils.NewConfig(ctx, utils.BlockRangeArgs)
	cfg.StateValidationMode = utils.SubsetCheck
	if argErr != nil {
		return argErr
	}

	// start CPU profiling if requested
	if profileFileName := ctx.String(utils.CpuProfileFlag.Name); profileFileName != "" {
		f, err := os.Create(profileFileName)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %s", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("could not start CPU profile: %s", err)
		}
		defer pprof.StopCPUProfile()
	}

	// open the archive
	db, err := openStateDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	// open substate DB
	substate.SetSubstateFlags(ctx)
	substate.OpenSubstateDBReadOnly()
	defer substate.CloseSubstateDB()

	log.Printf("Running transactions on archive using %d workers ...\n", cfg.Workers)
	iter := substate.NewSubstateIterator(cfg.First, cfg.Workers)
	defer iter.Release()

	if cfg.EnableProgress {
		start = time.Now()
		lastSec = time.Since(start).Seconds()
	}

	// Start a goroutine retrieving transactions and grouping them into blocks.
	blocks := make(chan []*substate.Transaction, 10*cfg.Workers)
	abort := make(chan bool, 1)
	go groupTransactions(iter, blocks, abort, cfg)

	// Start multiple workers processing transactions block by block.
	finishedTransaction := make(chan int, 10*cfg.Workers)
	finishedBlock := make(chan uint64, 10*cfg.Workers)
	issues := make(chan error, 10*cfg.Workers)
	dones := []<-chan bool{}
	for i := 0; i < cfg.Workers; i++ {
		done := make(chan bool)
		dones = append(dones, done)
		go runBlocks(db, blocks, finishedTransaction, finishedBlock, issues, done, cfg)
	}

	// Report progress while waiting for workers to complete.
	i := 0
	var lastBlock uint64
	for i < len(dones) {
		select {
		case issue := <-issues:
			err = issue
			// If an error is encountered, an abort is signaled.
			// But we need to keep consuming inputs until all workers are done.
			if abort != nil {
				close(abort)
				abort = nil
			}
		case <-finishedTransaction:
			if !cfg.EnableProgress {
				continue
			}
			txCount++
		case block := <-finishedBlock:
			if !cfg.EnableProgress {
				continue
			}
			if block > lastBlock {
				lastBlock = block
			}
			// Report progress on a regular time interval (wall time).
			sec = time.Since(start).Seconds()
			if sec-lastSec >= 15 {
				txRate := float64(txCount-lastTxCount) / (sec - lastSec)
				log.Printf("Elapsed time: %.0f s, at block %d (~ %.1f Tx/s)\n", sec, lastBlock, txRate)
				lastSec = sec
				lastTxCount = txCount
			}
		case <-dones[i]:
			i++
		}
	}

	// print progress summary
	if cfg.EnableProgress {
		runTime := time.Since(start).Seconds()
		log.Printf("Total elapsed time: %.3f s, processed %v blocks, %v transactions (~ %.1f Tx/s)\n", runTime, cfg.Last-cfg.First+1, txCount, float64(txCount)/(runTime))
	}

	return err
}

func openStateDB(cfg *utils.Config) (state.StateDB, error) {
	var err error

	if cfg.StateDbSrcDir == "" {
		return nil, fmt.Errorf("missing --db-src-dir parameter")
	}

	// check if statedb_info.json files exist
	dbInfoFile := filepath.Join(cfg.StateDbSrcDir, utils.DbInfoName)
	if _, err = os.Stat(dbInfoFile); err != nil {
		return nil, fmt.Errorf("%s does not appear to contain a state DB", cfg.StateDbSrcDir)
	}

	dbinfo, ferr := utils.ReadStateDbInfo(dbInfoFile)
	if ferr != nil {
		return nil, fmt.Errorf("failed to read %v. %v", dbInfoFile, ferr)
	}
	if dbinfo.Impl != cfg.DbImpl {
		err = fmt.Errorf("mismatch DB implementation.\n\thave %v\n\twant %v", dbinfo.Impl, cfg.DbImpl)
	} else if dbinfo.Variant != cfg.DbVariant {
		err = fmt.Errorf("mismatch DB variant.\n\thave %v\n\twant %v", dbinfo.Variant, cfg.DbVariant)
	} else if dbinfo.Block < cfg.Last {
		err = fmt.Errorf("the state DB does not cover the targeted block range.\n\thave %v\n\twant %v", dbinfo.Block, cfg.Last)
	} else if !dbinfo.ArchiveMode {
		err = fmt.Errorf("the targeted state DB does not include an archive")
	} else if dbinfo.ArchiveVariant != cfg.ArchiveVariant {
		err = fmt.Errorf("mismatch archive variant.\n\thave %v\n\twant %v", dbinfo.ArchiveVariant, cfg.ArchiveVariant)
	} else if dbinfo.Schema != cfg.CarmenSchema {
		err = fmt.Errorf("mismatch DB schema version.\n\thave %v\n\twant %v", dbinfo.Schema, cfg.CarmenSchema)
	}
	if err != nil {
		return nil, err
	}

	cfg.ArchiveMode = true
	return utils.MakeStateDB(cfg.StateDbSrcDir, cfg, dbinfo.RootHash, true)
}

func groupTransactions(iter substate.SubstateIterator, blocks chan<- []*substate.Transaction, abort <-chan bool, cfg *utils.Config) {
	defer close(blocks)
	var currentBlock uint64 = 0
	transactions := []*substate.Transaction{}
	for iter.Next() {
		select {
		case <-abort:
			return
		default:
			/* keep going */
		}
		tx := iter.Value()
		if tx.Block != currentBlock {
			if tx.Block > cfg.Last {
				break
			}
			currentBlock = tx.Block
			blocks <- transactions
			transactions = []*substate.Transaction{}
		}
		transactions = append(transactions, tx)
	}
	blocks <- transactions
}

func runBlocks(
	db state.StateDB,
	blocks <-chan []*substate.Transaction,
	transactionDone chan<- int,
	blockDone chan<- uint64,
	issues chan<- error,
	done chan<- bool,
	cfg *utils.Config) {
	var err error
	defer close(done)
	for transactions := range blocks {
		if len(transactions) == 0 {
			continue
		}
		block := transactions[0].Block
		var state state.StateDB
		if state, err = db.GetArchiveState(block - 1); err != nil {
			issues <- fmt.Errorf("failed to get state for block %d: %v", block, err)
			continue
		}

		state.BeginBlock(block)
		for _, tx := range transactions {
			state.BeginTransaction(uint32(tx.Transaction))
			if err = utils.ProcessTx(db, cfg, tx.Block, tx.Transaction, tx.Substate); err != nil {
				issues <- fmt.Errorf("processing of transaction %d/%d failed: %v", block, tx.Transaction, err)
				break
			}
			state.EndTransaction()
			transactionDone <- tx.Transaction
		}
		if err = state.Close(); err != nil {
			issues <- fmt.Errorf("failed to close state after block %d", block)
		}
		blockDone <- block
	}
}
