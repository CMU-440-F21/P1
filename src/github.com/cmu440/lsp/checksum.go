// DO NOT MODIFY THIS FILE!

package lsp

import (
	"encoding/binary"
)

// Int2Checksum calculates the 32-bit checksum for a given integer.
func Int2Checksum(value int) uint32 {
	return uint2Checksum(uint32(value))
}

func uint2Checksum(value uint32) uint32 {
	var sum uint32

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, value)
	lower := binary.LittleEndian.Uint16(buf[0:2])
	upper := binary.LittleEndian.Uint16(buf[2:4])
	sum += uint32(lower + upper)

	return sum
}

// ByteArray2Checksum calculates the 32-bit checksum for a given byte array.
func ByteArray2Checksum(value []byte) uint32 {
	var sum uint32

	numChunks := len(value)/2 + len(value)%2 // uint16 occupies 2 bytes

	lastIdx := len(value) - 1
	for i := 0; i < numChunks; i++ {
		startIdx := i * 2
		if startIdx < lastIdx {
			chunk := value[startIdx:(startIdx + 2)]
			tmp := binary.LittleEndian.Uint16(chunk)
			sum += uint32(tmp)
		} else {
			// Pad a zero byte at the end when the byte array has odd number of bytes
			chunk := []byte{value[startIdx], 0}
			tmp := binary.LittleEndian.Uint16(chunk)
			sum += uint32(tmp)
		}
	}

	return sum
}

// CalculateChecksum calculates the 16-bit checksum of the given fields
// for one data message.
func CalculateChecksum(connID, seqNum, size int, payload []byte) uint16 {
	var sum uint32
	var res uint16
	sum += Int2Checksum(connID)
	sum += Int2Checksum(seqNum)
	sum += Int2Checksum(size)
	sum += ByteArray2Checksum(payload)

	// Add upper 16-bit to lower 16-bit repeatedly,
	// until the sum fits in the lower 16-bit
	for sum > 0xffff {
		upperHalf := sum >> 16 & 0xffff
		lowerHalf := sum & 0xffff
		sum = upperHalf + lowerHalf
	}

	// Take one's complement of the final sum
	res = ^uint16(sum)
	return res
}
