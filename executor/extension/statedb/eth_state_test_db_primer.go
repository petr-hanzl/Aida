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

package statedb

import (
	"github.com/Fantom-foundation/Aida/executor"
	"github.com/Fantom-foundation/Aida/executor/extension"
	"github.com/Fantom-foundation/Aida/logger"
	"github.com/Fantom-foundation/Aida/txcontext"
	"github.com/Fantom-foundation/Aida/utils"
)

func MakeEthStateTestDbPrimer(cfg *utils.Config) executor.Extension[txcontext.TxContext] {
	return makeEthStateTestDbPrimer(logger.NewLogger(cfg.LogLevel, "EthStatePrimer"), cfg)
}

func makeEthStateTestDbPrimer(log logger.Logger, cfg *utils.Config) *ethStateTestDbPrimer {
	return &ethStateTestDbPrimer{
		cfg: cfg,
		log: log,
	}
}

type ethStateTestDbPrimer struct {
	extension.NilExtension[txcontext.TxContext]
	cfg *utils.Config
	log logger.Logger
}

func (e ethStateTestDbPrimer) PreTransaction(st executor.State[txcontext.TxContext], ctx *executor.Context) error {
	primeCtx := utils.NewPrimeContext(e.cfg, ctx.State, e.log)
	return primeCtx.PrimeStateDB(st.Data.GetInputState(), ctx.State)
}
