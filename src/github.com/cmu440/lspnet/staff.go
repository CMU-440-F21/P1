// DO NOT MODIFY THIS FILE!
// STUDENTS MUST NOT CALL ANY METHODS IN THIS FILE!

package lspnet

import "sync/atomic"

var (
	clientReadDropPercent  uint32
	clientWriteDropPercent uint32
	serverReadDropPercent  uint32
	serverWriteDropPercent uint32
	msgShorteningPercent   uint32
	msgLengtheningPercent  uint32
	delayMessagePercent    uint32
	corruptedMessage       uint32
)

// SetReadDropPercent sets the read drop percent for both clients and servers.
func SetReadDropPercent(p int) {
	SetClientReadDropPercent(p)
	SetServerReadDropPercent(p)
}

// SetWriteDropPercent sets the write drop percent for clients and servers.
func SetWriteDropPercent(p int) {
	SetClientWriteDropPercent(p)
	SetServerWriteDropPercent(p)
}

// SetMsgShorteningPercent sets the message shortening percent for clients and servers.
func SetMsgShorteningPercent(p int) {
	if 0 <= 0 && p <= 100 {
		atomic.StoreUint32(&msgShorteningPercent, uint32(p))
	}
}

// SetMsgLengtheningPercent sets the message lengthening percent for clients and servers.
func SetMsgLengtheningPercent(p int) {
	if 0 <= 0 && p <= 100 {
		atomic.StoreUint32(&msgLengtheningPercent, uint32(p))
	}
}

// SetMsgCorrupted sets the message corruption flag for clients and servers.
func SetMsgCorrupted(corrupted bool) {
	if corrupted {
		atomic.StoreUint32(&corruptedMessage, 1)
	} else {
		atomic.StoreUint32(&corruptedMessage, 0)
	}
}

// SetClientReadDropPercent sets the read drop percent for clients.
func SetClientReadDropPercent(p int) {
	if 0 <= p && p <= 100 {
		atomic.StoreUint32(&clientReadDropPercent, uint32(p))
	}
}

// SetClientWriteDropPercent sets the write drop percent for clients.
func SetClientWriteDropPercent(p int) {
	if 0 <= p && p <= 100 {
		atomic.StoreUint32(&clientWriteDropPercent, uint32(p))
	}
}

// SetServerReadDropPercent sets the read drop percent for servers.
func SetServerReadDropPercent(p int) {
	if 0 <= p && p <= 100 {
		atomic.StoreUint32(&serverReadDropPercent, uint32(p))
	}
}

// SetServerWriteDropPercent sets the write drop percent for servers.
func SetServerWriteDropPercent(p int) {
	if 0 <= p && p <= 100 {
		atomic.StoreUint32(&serverWriteDropPercent, uint32(p))
	}
}

// ResetDropPercent resets all drop percents to 0.
func ResetDropPercent() {
	SetReadDropPercent(0)
	SetWriteDropPercent(0)
}

func readDropPercent(c *UDPConn) int {
	mapMutex.Lock()
	isServer, ok := connectionMap[*c]
	mapMutex.Unlock()
	if ok && isServer {
		return int(atomic.LoadUint32(&serverReadDropPercent))
	} else if ok && !isServer {
		return int(atomic.LoadUint32(&clientReadDropPercent))
	}
	return 0 // This shouldn't happen, but just in case...
}

func writeDropPercent(c *UDPConn) int {
	mapMutex.Lock()
	isServer, ok := connectionMap[*c]
	mapMutex.Unlock()
	if ok && isServer {
		return int(atomic.LoadUint32(&serverWriteDropPercent))
	} else if ok && !isServer {
		return int(atomic.LoadUint32(&clientWriteDropPercent))
	}
	return 0 // This shouldn't happen, but just in case...
}

func SetDelayMessagePercent(p int) {
	if 0 <= 0 && p <= 100 {
		atomic.StoreUint32(&delayMessagePercent, uint32(p))
	}
}
