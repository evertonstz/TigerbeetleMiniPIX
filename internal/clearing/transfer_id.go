package clearing

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// GenerateTransferID produces a deterministic uint128 transfer ID
// using SHA-256(paymentUUID + legIndex + phaseSuffix).
// Returns the first 8 bytes of SHA-256 as uint64 (upper half of uint128).
func GenerateTransferID(paymentUUID string, legIndex int, phaseSuffix string) uint64 {
	data := fmt.Sprintf("%s:leg%d:%s", paymentUUID, legIndex, phaseSuffix)
	hash := sha256.Sum256([]byte(data))

	id := binary.BigEndian.Uint64(hash[:8])

	if id == 0 {
		id = 1
	}

	return id
}
