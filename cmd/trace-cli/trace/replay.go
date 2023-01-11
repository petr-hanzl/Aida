package trace

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/Fantom-foundation/Aida/tracer"
	"github.com/Fantom-foundation/Aida/tracer/dict"
	"github.com/Fantom-foundation/Aida/tracer/operation"
	"github.com/ethereum/go-ethereum/substate"
	"github.com/urfave/cli/v2"
)

// TraceReplayCommand data structure for the replay app
var TraceReplayCommand = cli.Command{
	Action:    traceReplayAction,
	Name:      "replay",
	Usage:     "executes storage trace",
	ArgsUsage: "<blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		&chainIDFlag,
		&cpuProfileFlag,
		&deletedAccountDirFlag,
		&disableProgressFlag,
		&epochLengthFlag,
		&memoryBreakdownFlag,
		&memProfileFlag,
		&primeSeedFlag,
		&primeThresholdFlag,
		&profileFlag,
		&randomizePrimingFlag,
		&stateDbImplementationFlag,
		&stateDbVariantFlag,
		&stateDbTempDirFlag,
		&stateDbLoggingFlag,
		&shadowDbImplementationFlag,
		&shadowDbVariantFlag,
		&substate.SubstateDirFlag,
		&substate.WorkersFlag,
		&traceDirectoryFlag,
		&traceDebugFlag,
		&updateDBDirFlag,
		&validateFlag,
		&validateWorldStateFlag,
	},
	Description: `
The trace replay command requires two arguments:
<blockNumFirst> <blockNumLast>

<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to replay storage traces.`,
}

// readTrace reads operations from trace files and puts them into a channel.
func readTrace(cfg *TraceConfig, ch chan operation.Operation) {
	traceIter := tracer.NewTraceIterator(cfg.first, cfg.last)
	defer traceIter.Release()
	for traceIter.Next() {
		op := traceIter.Value()
		ch <- op
	}
	close(ch)
}

// traceReplayTask simulates storage operations from storage traces on stateDB.
func traceReplayTask(cfg *TraceConfig) error {

	// starting reading in parallel
	log.Printf("Start reading operations in parallel")
	opChannel := make(chan operation.Operation, 100000)
	go readTrace(cfg, opChannel)

	// load dictionaries & indexes
	log.Printf("Load dictionaries")
	dCtx := dict.ReadDictionaryContext()

	// create a directory for the store to place all its files, and
	// instantiate the state DB under testing.
	log.Printf("Create stateDB database")
	stateDirectory, err := ioutil.TempDir(cfg.stateDbDir, "state_db_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stateDirectory)
	log.Printf("\tTemporary state DB directory: %v\n", stateDirectory)
	db, err := MakeStateDB(stateDirectory, cfg)
	if err != nil {
		return err
	}

	// intialize the world state and advance it to the first block
	log.Printf("Load and advance worldstate to block %v", cfg.first-1)
	ws, err := generateWorldStateFromUpdateDB(cfg, cfg.first-1)
	if err != nil {
		return err
	}

	// prime stateDB
	log.Printf("Prime stateDB \n")
	primeStateDB(ws, db, cfg)

	// print memory usage after priming
	if cfg.memoryBreakdown {
		if usage := db.GetMemoryUsage(); usage != nil {
			log.Printf("State DB memory usage: %d byte\n%s\n", usage.UsedBytes, usage.Breakdown)
		} else {
			log.Printf("Utilized storage solution does not support memory breakdowns.\n")
		}
	}

	// Release world state to free memory.
	ws = substate.SubstateAlloc{}

	// delete destroyed accounts from stateDB
	log.Printf("Delete destroyed accounts \n")
	// remove destroyed accounts until one block before the first block
	if err = deleteDestroyedAccountsFromStateDB(db, cfg, cfg.first-1); err != nil {
		return err
	}

	log.Printf("Replay storage operations on StateDB database")

	// progress message setup
	var (
		start      time.Time
		sec        float64
		lastSec    float64
		firstBlock = true
	)
	if cfg.enableProgress {
		start = time.Now()
		sec = time.Since(start).Seconds()
		lastSec = time.Since(start).Seconds()
	}

	// A utility to run operations on the local context.
	run := func(op operation.Operation) {
		operation.Execute(op, db, dCtx)
		if cfg.debug {
			operation.Debug(dCtx, op)
		}
	}

	// replay storage trace
	for op := range opChannel {
		if beginBlock, ok := op.(*operation.BeginBlock); ok {
			block := beginBlock.BlockNumber

			// The first Epoch begin and the final EpochEnd need to be artificially
			// added since the range running on may not match epoch boundaries.
			if firstBlock {
				run(operation.NewBeginEpoch(cfg.first / cfg.epochLength))
				firstBlock = false
			}

			if block > cfg.last {
				run(operation.NewEndEpoch())
				break
			}
			if cfg.enableProgress {
				// report progress
				sec = time.Since(start).Seconds()
				if sec-lastSec >= 15 {
					log.Printf("Elapsed time: %.0f s, at block %v\n", sec, block)
					lastSec = sec
				}
			}
		}
		run(op)
	}

	sec = time.Since(start).Seconds()

	log.Printf("Finished replaying storage operations on StateDB database")

	// validate stateDB
	if cfg.validateWorldState {
		log.Printf("Validate final state")
		ws, err := generateWorldStateFromUpdateDB(cfg, cfg.last)
		if err = deleteDestroyedAccountsFromWorldState(ws, cfg, cfg.last); err != nil {
			return fmt.Errorf("Failed to remove detroyed accounts. %v\n", err)
		}
		if err := validateStateDB(ws, db, false); err != nil {
			return fmt.Errorf("Validation failed. %v\n", err)
		}
	}

	if cfg.memoryBreakdown {
		if usage := db.GetMemoryUsage(); usage != nil {
			log.Printf("State DB memory usage: %d byte\n%s\n", usage.UsedBytes, usage.Breakdown)
		} else {
			log.Printf("Utilized storage solution does not support memory breakdowns.\n")
		}
	}

	// print profile statistics (if enabled)
	if operation.EnableProfiling {
		operation.PrintProfiling()
	}

	// close the DB and print disk usage
	log.Printf("Close StateDB database")
	start = time.Now()
	if err := db.Close(); err != nil {
		log.Printf("Failed to close database: %v", err)
	}

	// print progress summary
	if cfg.enableProgress {
		log.Printf("trace replay: Total elapsed time: %.3f s, processed %v blocks\n", sec, cfg.last-cfg.first+1)
		log.Printf("trace replay: Closing DB took %v\n", time.Since(start))
		log.Printf("trace replay: Final disk usage: %v MiB\n", float32(getDirectorySize(stateDirectory))/float32(1024*1024))
	}

	return nil
}

// traceReplayAction implements trace command for replaying.
func traceReplayAction(ctx *cli.Context) error {
	var err error
	cfg, err := NewTraceConfig(ctx, blockRangeArgs)
	if err != nil {
		return err
	}
	if cfg.dbImpl == "memory" {
		return fmt.Errorf("db-impl memory is not supported")
	}

	operation.EnableProfiling = cfg.profile
	// set trace directory
	tracer.TraceDir = ctx.String(traceDirectoryFlag.Name) + "/"
	dict.DictionaryContextDir = ctx.String(traceDirectoryFlag.Name) + "/"

	// start CPU profiling if requested.
	if profileFileName := ctx.String(cpuProfileFlag.Name); profileFileName != "" {
		f, err := os.Create(profileFileName)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %s", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("could not start CPU profile: %s", err)
		}
		defer pprof.StopCPUProfile()
	}

	// run storage driver
	substate.SetSubstateFlags(ctx)
	substate.OpenSubstateDBReadOnly()
	defer substate.CloseSubstateDB()
	err = traceReplayTask(cfg)

	// write memory profile if requested
	if profileFileName := ctx.String(memProfileFlag.Name); profileFileName != "" && err == nil {
		f, err := os.Create(profileFileName)
		if err != nil {
			return fmt.Errorf("could not create memory profile: %s", err)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			return fmt.Errorf("could not write memory profile: %s", err)
		}
	}

	return err
}
