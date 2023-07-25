package db

import (
	"log"
	"os"

	"github.com/Fantom-foundation/Aida/logger"
	"github.com/Fantom-foundation/Aida/utils"
	substate "github.com/Fantom-foundation/Substate"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/op/go-logging"
	"github.com/urfave/cli/v2"
)

// AutoGenCommand generates aida-db patches and handles second opera for event generation
var AutoGenCommand = cli.Command{
	Action: autogen,
	Name:   "autogen",
	Usage:  "autogen generates aida-db periodically",
	Flags: []cli.Flag{
		// TODO minimal epoch length for patch generation
		&utils.AidaDbFlag,
		&utils.ChainIDFlag,
		&utils.DbFlag,
		&utils.GenesisFlag,
		&utils.DbTmpFlag,
		&utils.UpdateBufferSizeFlag,
		&utils.OutputFlag,
		&utils.WorldStateFlag,
		&substate.WorkersFlag,
		&logger.LogLevelFlag,
	},
	Description: `
AutoGen generates aida-db patches and handles second opera for event generation. Generates event file, which is supplied into doGenerations to create aida-db patch.
`,
}

// autogen command is used to record/update aida-db periodically
func autogen(ctx *cli.Context) error {
	cfg, err := utils.NewConfig(ctx, utils.NoArgs)
	if err != nil {
		return err
	}

	g, err := newGenerator(ctx, cfg)
	if err != nil {
		return err
	}

	err = g.opera.init()
	if err != nil {
		return err
	}

	// remove worldstate directory if it was created
	defer func(log *logging.Logger) {
		if cfg.WorldStateDb != "" {
			err = os.RemoveAll(cfg.WorldStateDb)
			if err != nil {
				log.Criticalf("can't remove temporary folder: %v; %v", cfg.WorldStateDb, err)
			}
		}
	}(g.log)

	err = g.calculatePatchEnd()
	if err != nil {
		return err
	}

	g.log.Noticef("Starting substate generation %d - %d", g.opera.lastEpoch+1, g.stopAtEpoch)

	MustCloseDB(g.aidaDb)

	// stop opera to be able to export events
	errCh := startOperaRecording(g.cfg, g.stopAtEpoch)

	// wait for opera recording response
	err, ok := <-errCh
	if ok && err != nil {
		return err
	}
	g.log.Noticef("Opera %v - successfully substates for epoch range %d - %d", g.cfg.Db, g.opera.lastEpoch+1, g.stopAtEpoch)

	// reopen aida-db
	g.aidaDb, err = rawdb.NewLevelDBDatabase(cfg.AidaDb, 1024, 100, "profiling", false)
	if err != nil {
		log.Fatalf("cannot create new db; %v", err)
		return err
	}
	substate.SetSubstateDbBackend(g.aidaDb)

	err = g.opera.getOperaBlockAndEpoch(false)
	if err != nil {
		return err
	}

	return g.Generate()
}
