package clearing

import (
	"sync"
)

type OffsetManager struct {
	offsets map[string]map[int32]int64
	mu      sync.RWMutex
}

func NewOffsetManager() *OffsetManager {
	return &OffsetManager{
		offsets: make(map[string]map[int32]int64),
	}
}

func (om *OffsetManager) MarkForCommit(topic string, partition int32, offset int64) {
	om.mu.Lock()
	defer om.mu.Unlock()

	if _, exists := om.offsets[topic]; !exists {
		om.offsets[topic] = make(map[int32]int64)
	}

	om.offsets[topic][partition] = offset
}

func (om *OffsetManager) GetLastCommitted(topic string, partition int32) (int64, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	if topicOffsets, exists := om.offsets[topic]; exists {
		if offset, exists := topicOffsets[partition]; exists {
			return offset, true
		}
	}

	return 0, false
}

func (om *OffsetManager) GetAllOffsets() map[string]map[int32]int64 {
	om.mu.RLock()
	defer om.mu.RUnlock()

	result := make(map[string]map[int32]int64)
	for topic, partitions := range om.offsets {
		result[topic] = make(map[int32]int64)
		for partition, offset := range partitions {
			result[topic][partition] = offset
		}
	}
	return result
}
