package simulation

import (
	"crypto/sha256"
	"math/rand"
)

// Generator wraps a Label of the distribution and a function to get a next value withing the given distribution
type Generator struct {
	C      []float32
	Size   uint32
	GetNew func() []any
	E      float64
}

var hasher = sha256.New()

func (g Generator) GetNext(opId byte) []any {
	nc := rand.Float32()
	if nc <= g.C[opId] {
		//	generating new value
		return g.GetNew()
	} else {
		//	using existing value
		return []any{g.getExisting()}
	}
}

func (g Generator) getExisting() uint32 {
	var expRate float64
	if g.E != 0 {
		expRate = g.E
	} else {
		expRate = float64(10) / float64(g.Size)
	}

	return uint32(rand.ExpFloat64()/expRate) % g.Size
}

// i32tob convert uint32 to []byte
func i32tob(val uint32) []byte {
	r := make([]byte, 4)
	for i := uint32(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}

// generates 32B deterministically
func RetrieveValueAt(i uint32) []byte {
	hasher.Reset()
	hasher.Write(i32tob(i))
	return hasher.Sum(nil)
}
