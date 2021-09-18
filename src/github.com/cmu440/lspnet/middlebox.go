// DO NOT MODIFY THIS FILE!
// STUDENTS MUST NOT CALL ANY METHODS IN THIS FILE!

package lspnet

import (
	"sync"
	"sync/atomic"
)

type MiddleboxOutput struct {
	SendMsg     bool // True is message should be sent, false otherwise
	ModifiedMsg bool // True if message was modified, false otherwise
}

type MiddleboxInterface interface {
	Run(msg *TemporaryMessage) MiddleboxOutput
}

var middleboxLock sync.Mutex
var middleboxStarted uint32 = 0
var middleboxImpl MiddleboxInterface = nil

func isMiddleboxStarted() bool {
	if atomic.LoadUint32(&middleboxStarted) == 0 {
		return false
	}
	return true
}

func runMiddlebox(msg *TemporaryMessage) MiddleboxOutput {
	middleboxLock.Lock()
	defer middleboxLock.Unlock()
	return middleboxImpl.Run(msg)
}

func StartMiddlebox(m MiddleboxInterface) {
	middleboxLock.Lock()
	middleboxImpl = m
	middleboxLock.Unlock()
	atomic.StoreUint32(&middleboxStarted, 1)
}

func StopMiddlebox() {
	atomic.StoreUint32(&middleboxStarted, 0)
	middleboxLock.Lock()
	middleboxImpl = nil
	defer middleboxLock.Unlock()
}
