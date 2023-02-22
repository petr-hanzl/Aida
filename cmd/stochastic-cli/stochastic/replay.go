package stochastic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Fantom-foundation/Aida/stochastic"
	"github.com/Fantom-foundation/Aida/utils"
	"github.com/urfave/cli/v2"
)

// StochasticReplayCommand data structure for the replay app.
var StochasticReplayCommand = cli.Command{
	Action:    stochasticReplayAction,
	Name:      "replay",
	Usage:     "Simulates StateDB operations using a random generator with realistic distributions",
	ArgsUsage: "<simulation-file>",
	Flags: []cli.Flag{
		&utils.VerboseFlag,
		&utils.ChainIDFlag, // TODO: this flag does not make sense for stochastic replay/remove later
		&utils.CpuProfileFlag,
		&utils.DisableProgressFlag,
		&utils.EpochLengthFlag,
		&utils.KeepStateDBFlag,
		&utils.MemoryBreakdownFlag,
		&utils.MemProfileFlag,
		&utils.ProfileFlag,
		&utils.StateDbImplementationFlag,
		&utils.StateDbVariantFlag,
		&utils.StateDbSrcDirFlag,
		&utils.StateDbTempDirFlag,
		&utils.StateDbLoggingFlag,
		&utils.ShadowDbImplementationFlag,
		&utils.ShadowDbVariantFlag,
	},
	Description: `
The stochastic replay command requires two argument:
<simulation-length> <simulation.json> 

<simulation-length> determines the number of blocks
<simulation.json> contains the simulation parameters produced by the stochastic estimator.`,
}

// stochasticReplayAction implements the replay command. The user
// provides simulation file and simulation as arguments.
func stochasticReplayAction(ctx *cli.Context) error {
	// parse command-line arguments
	if ctx.Args().Len() != 2 {
		return fmt.Errorf("missing simulation file and simulation length as parameter")
	}
	simLength, perr := strconv.ParseInt(ctx.Args().Get(0), 10, 64)
	if perr != nil {
		return fmt.Errorf("simulation length is not an integer. Error: %v", perr)
	}

	// read simulation file
	simulation, serr := readSimulation(ctx.Args().Get(1))
	if serr != nil {
		return fmt.Errorf("failed reading simulation. Error: %v", serr)
	}

	// process configuration
	cfg, err := utils.NewConfig(ctx, utils.LastBlockArg)
	if err != nil {
		return err
	}
	if cfg.DbImpl == "memory" {
		return fmt.Errorf("db-impl memory is not supported")
	}

	// create a directory for the store to place all its files, and
	// instantiate the state DB under testing.
	log.Printf("Create stateDB database")
	db, stateDirectory, _, err := utils.PrepareStateDB(cfg)
	if err != nil {
		return err
	}
	if !cfg.KeepStateDB {
		log.Printf("WARNING: directory %v will be removed at the end of this run.\n", stateDirectory)
		defer os.RemoveAll(stateDirectory)
	}

	// run simulation.
	fmt.Printf("stochastic replay: run simulation ...\n")
	verbose := ctx.Bool(utils.VerboseFlag.Name)
	stochastic.RunStochasticReplay(db, simulation, int(simLength), verbose)

	// print memory usage after simulation
	if cfg.MemoryBreakdown {
		log.Printf("Utilized storage solution does not support memory breakdowns.\n")
	}

	// close the DB and print disk usage
	start := time.Now()
	if err := db.Close(); err != nil {
		log.Printf("Failed to close database: %v", err)
	}
	log.Printf("stochastic replay: Closing DB took %v\n", time.Since(start))
	log.Printf("stochastic replay: Final disk usage: %v MiB\n", float32(utils.GetDirectorySize(stateDirectory))/float32(1024*1024))
	if usage := db.GetMemoryUsage(); usage != nil {
		log.Printf("stochastic replay: state DB memory usage: %d byte\n%s\n", usage.UsedBytes, usage.Breakdown)
	}

	return nil
}

// readSimulation reads the simulation file in JSON format (generated by the estimator).
func readSimulation(filename string) (*stochastic.EstimationModelJSON, error) {
	// open simulation file and read JSON
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed opening simulation file")
	}
	defer file.Close()

	// read file into memory
	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed reading simulation file")
	}

	// convert text to JSON object
	var simulation stochastic.EstimationModelJSON
	err = json.Unmarshal(contents, &simulation)
	if err != nil {
		return nil, fmt.Errorf("failed unmarshalling JSON")
	}

	return &simulation, nil
}
