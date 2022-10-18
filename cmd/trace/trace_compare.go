package trace

import (
	"fmt"
	"github.com/ethereum/go-ethereum/substate"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"os"
	"regexp"
)

// Trace compare-log command
var TraceCompareLogCommand = cli.Command{
	Action:    traceCompareLogAction,
	Name:      "compare-log",
	Usage:     "compares storage debug log between record and replay",
	ArgsUsage: "<blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		&chainIDFlag,
		&stateDbImplementation,
		&substate.SubstateDirFlag,
		&substate.WorkersFlag,
		&traceDebugFlag,
		&traceDirectoryFlag,
	},
	Description: `
The trace compare-log command requires two arguments:
<blockNumFirst> <blockNumLast>

<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to replay storage traces.`,
}

// captureDebugLog captures debug log in a string buffer.
func captureDebugLog(traceFunc func(*cli.Context) error, ctx *cli.Context) (string, error) {
	defer func(stdout *os.File) {
		os.Stdout = stdout
	}(os.Stdout)

	// create tmp file storing debug trances
	tmpfile, fileErr := os.CreateTemp("", "debug_trace_tmp")
	if fileErr != nil {
		return "", fileErr
	}
	tmpname := tmpfile.Name()
	// remove tmpfile
	defer os.Remove(tmpname)

	// redirect stdout to tmp file
	os.Stdout = tmpfile

	// run trace record/replay
	err := traceFunc(ctx)

	fileErr = tmpfile.Close()
	if fileErr != nil {
		return "", fileErr
	}
	// copy the output from tmp file
	debugMessage, fileErr := ioutil.ReadFile(tmpname)
	if fileErr != nil {
		return "", fileErr
	}

	return string(debugMessage), err
}

// isLogEqual returns true if input debug traces are identical.
func isLogEqual(record string, replay string) bool {
	re := regexp.MustCompile("(?m)[\r\n]+^.*record-replay.*$")
	record = re.ReplaceAllString(record, "")
	replay = re.ReplaceAllString(replay, "")
	return record == replay
}

// traceCompareLogAction implements trace command for validating record and replay debug log.
func traceCompareLogAction(ctx *cli.Context) error {
	// process arguments
	if ctx.Args().Len() != 2 {
		return fmt.Errorf("trace replay-trace command requires exactly 2 arguments")
	}

	// enable debug-trace
	if !ctx.IsSet(traceDebugFlag.Name) {
		ctxErr := ctx.Set(traceDebugFlag.Name, "true")
		if ctxErr != nil {
			return ctxErr
		}
	}
	fmt.Printf("Capture record trace\n")
	recordLog, recErr := captureDebugLog(traceRecordAction, ctx)
	if recErr != nil {
		return recErr
	}
	fmt.Printf("Capture replay trace\n")
	replayLog, repErr := captureDebugLog(traceReplayAction, ctx)
	if repErr != nil {
		return recErr
	}

	if !isLogEqual(recordLog, replayLog) {
		return fmt.Errorf("Replay trace doesn't match record trace.")
	} else {
		fmt.Printf("Replay trace matches record trace.\n")
	}

	return nil
}
