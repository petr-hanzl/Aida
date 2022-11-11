package operation

import (
	"fmt"
	"github.com/Fantom-foundation/Aida/tracer/dict"
	"testing"
)

func initSnapshot(t *testing.T) (*dict.DictionaryContext, *Snapshot, int32) {
	// create dictionary context
	dict := dict.NewDictionaryContext()

	var snapID int32 = 1
	// create new operation
	op := NewSnapshot(snapID)
	if op == nil {
		t.Fatalf("failed to create operation")
	}
	// check id
	if op.GetId() != SnapshotID {
		t.Fatalf("wrong ID returned")
	}
	return dict, op, snapID
}

// TestSnapshotReadWrite writes a new Snapshot object into a buffer, reads from it,
// and checks equality.
func TestSnapshotReadWrite(t *testing.T) {
	_, op1, _ := initSnapshot(t)
	testOperationReadWrite(t, op1, ReadSnapshot)
}

// TestSnapshotDebug creates a new Snapshot object and checks its Debug message.
func TestSnapshotDebug(t *testing.T) {
	dict, op, snapID := initSnapshot(t)
	testOperationDebug(t, dict, op, fmt.Sprint(snapID))
}

// TestSnapshotExecute
func TestSnapshotExecute(t *testing.T) {
	dict, op, _ := initSnapshot(t)

	// check execution
	mock := NewMockStateDB()
	op.Execute(mock, dict)

	// check whether methods were correctly called
	expected := []Record{{SnapshotID, nil}}
	mock.compareRecordings(expected, t)
}