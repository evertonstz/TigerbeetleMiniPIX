package clearing

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOffsetManagerTracksCommitState(t *testing.T) {
	manager := NewOffsetManager()

	// Initially no offsets
	offset, ok := manager.GetLastCommitted("pix-payments", 0)
	assert.False(t, ok)
	assert.Equal(t, int64(0), offset)

	// Mark an offset
	manager.MarkForCommit("pix-payments", 0, 100)
	offset, ok = manager.GetLastCommitted("pix-payments", 0)
	assert.True(t, ok)
	assert.Equal(t, int64(100), offset)

	// Update to newer offset
	manager.MarkForCommit("pix-payments", 0, 101)
	offset, ok = manager.GetLastCommitted("pix-payments", 0)
	assert.True(t, ok)
	assert.Equal(t, int64(101), offset)
}

func TestOffsetManagerHandlesMultiplePartitions(t *testing.T) {
	manager := NewOffsetManager()

	manager.MarkForCommit("pix-payments", 0, 100)
	manager.MarkForCommit("pix-payments", 1, 50)

	offset0, _ := manager.GetLastCommitted("pix-payments", 0)
	offset1, _ := manager.GetLastCommitted("pix-payments", 1)

	assert.Equal(t, int64(100), offset0)
	assert.Equal(t, int64(50), offset1)
}
