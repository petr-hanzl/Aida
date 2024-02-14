package executor

import (
	"github.com/Fantom-foundation/Aida/rpc"
	"github.com/Fantom-foundation/Aida/tracer/operation"
	"github.com/Fantom-foundation/Aida/txcontext"
)

//go:generate mockgen -source test_consumer.go -destination test_consumer_mocks.go -package executor

//---------------------------------------------------------------------------------//
// This file serves for creating a mock Consumer with specific type. Every possible
// type of Consumer should be included therefore should be tested independently.
//---------------------------------------------------------------------------------//

type TxConsumer interface {
	Consume(block int, transaction int, substate txcontext.TxContext) error
}

func toSubstateConsumer(c TxConsumer) Consumer[txcontext.TxContext] {
	return func(info TransactionInfo[txcontext.TxContext]) error {
		return c.Consume(info.Block, info.Transaction, info.Data)
	}
}

type RPCReqConsumer interface {
	Consume(block int, transaction int, req *rpc.RequestAndResults) error
}

func toRPCConsumer(c RPCReqConsumer) Consumer[*rpc.RequestAndResults] {
	return func(info TransactionInfo[*rpc.RequestAndResults]) error {
		return c.Consume(info.Block, info.Transaction, info.Data)
	}
}

type OperationConsumer interface {
	Consume(block int, transaction int, operations []operation.Operation) error
}

func toOperationConsumer(c OperationConsumer) Consumer[[]operation.Operation] {
	return func(info TransactionInfo[[]operation.Operation]) error {
		return c.Consume(info.Block, info.Transaction, info.Data)
	}
}