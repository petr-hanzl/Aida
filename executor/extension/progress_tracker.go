package extension

import (
	"sync"
	"time"

	"github.com/Fantom-foundation/Aida/executor"
	"github.com/Fantom-foundation/Aida/logger"
	"github.com/Fantom-foundation/Aida/utils"
)

const (
	ProgressTrackerDefaultReportFrequency = 100_000 // in blocks
	progressTrackerReportFormat           = "Reached block %d; using ~ %v bytes of memory, ~ %v bytes of disk, last interval rate ~ %v Tx/s, ~ %v Gas/s"
)

// MakeProgressTracker creates a progressTracker that depends on the
// PostBlock event and is only useful as part of a sequential evaluation.
func MakeProgressTracker(config *utils.Config, reportFrequency int) executor.Extension {
	if !config.TrackProgress {
		return NilExtension{}
	}

	if reportFrequency == 0 {
		reportFrequency = ProgressTrackerDefaultReportFrequency
	}

	return makeProgressTracker(config, reportFrequency, logger.NewLogger(config.LogLevel, "ProgressTracker"))
}

func makeProgressTracker(config *utils.Config, reportFrequency int, log logger.Logger) *progressTracker {
	return &progressTracker{
		config:            config,
		log:               log,
		reportFrequency:   reportFrequency,
		lastReportedBlock: int(config.First) - (int(config.First) % reportFrequency),
	}
}

// progressTracker logs progress every XXX blocks depending on reportFrequency.
// Default is 100_000 blocks. This is mainly used for gathering information about process.
type progressTracker struct {
	NilExtension
	config              *utils.Config
	log                 logger.Logger
	reportFrequency     int
	lastReportedBlock   int
	startOfLastInterval time.Time
	lastIntervalInfo    processInfo
	lock                sync.Mutex
}

type processInfo struct {
	numTransactions uint64
	gas             uint64
}

func (t *progressTracker) PreRun(_ executor.State) error {
	t.startOfLastInterval = time.Now()
	return nil
}

// PostTransaction increments number of transactions and saves gas used in last substate.
func (t *progressTracker) PostTransaction(state executor.State) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.lastIntervalInfo.numTransactions++
	t.lastIntervalInfo.gas += state.Substate.Result.GasUsed

	return nil
}

// PostBlock sends the state to the report goroutine.
// We only care about total number of transactions we can do this here rather in PostTransaction.
func (t *progressTracker) PostBlock(state executor.State) error {
	boundary := state.Block - (state.Block % t.reportFrequency)

	if state.Block-t.lastReportedBlock < t.reportFrequency {
		return nil
	}

	elapsed := time.Since(t.startOfLastInterval)

	// quickly extract interval info and reset its values
	t.lock.Lock()
	info := t.lastIntervalInfo
	t.lastIntervalInfo.gas = 0
	t.lastIntervalInfo.numTransactions = 0
	t.lock.Unlock()

	disk := utils.GetDirectorySize(t.config.StateDbSrc)
	m := state.State.GetMemoryUsage()

	memory := uint64(0)
	if m != nil {
		memory = m.UsedBytes
	}

	txRate := float64(info.numTransactions) / elapsed.Seconds()
	gasRate := float64(info.gas) / elapsed.Seconds()

	t.log.Noticef(progressTrackerReportFormat, boundary, disk, memory, txRate, gasRate)

	t.lastReportedBlock = boundary
	t.startOfLastInterval = time.Now()

	return nil
}
