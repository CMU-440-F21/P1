// DO NOT MODIFY THIS FILE!
// STUDENTS MUST NOT CALL ANY METHODS IN THIS FILE!

package lspnet

import (
	"sync"
	"sync/atomic"
)

type SniffResult struct {
	NumSentACKs    int
	NumDroppedACKS int
	NumSentData    int
	NumDroppedData int
	AllMessages    []*TemporaryMessage
	SentMessages   []*TemporaryMessage
}

var isSniffing uint32 = 0
var sniffRes SniffResult
var sniffResLock sync.Mutex

func isSniff() bool {
	if atomic.LoadUint32(&isSniffing) == 0 {
		return false
	}
	return true
}

func record(msg *TemporaryMessage, isSent bool) {
	sniffResLock.Lock()
	defer sniffResLock.Unlock()
	sniffRes.AllMessages = append(sniffRes.AllMessages, msg)
	if isSent {
		sniffRes.SentMessages = append(sniffRes.SentMessages, msg)
	}

	if msg.Type == TypeMsgData {
		if isSent {
			sniffRes.NumSentData++
		} else {
			sniffRes.NumDroppedData++
		}
	} else if msg.Type == TypeMsgAck {
		if isSent {
			sniffRes.NumSentACKs++
		} else {
			sniffRes.NumDroppedACKS++
		}
	}
}

func StartSniff() {
	sniffResLock.Lock()
	sniffRes.NumSentACKs = 0
	sniffRes.NumDroppedACKS = 0
	sniffRes.NumSentData = 0
	sniffRes.NumDroppedData = 0
	sniffRes.AllMessages = []*TemporaryMessage{}
	sniffRes.SentMessages = []*TemporaryMessage{}
	sniffResLock.Unlock()
	atomic.StoreUint32(&isSniffing, 1)
}

func StopSniff() SniffResult {
	atomic.StoreUint32(&isSniffing, 0)
	sniffResLock.Lock()
	defer sniffResLock.Unlock()
	return sniffRes
}
