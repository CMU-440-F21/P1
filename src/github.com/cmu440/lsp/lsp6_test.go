package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

// Message lengths
const (
	SHORT = iota
	NORMAL
	LONG
)

/* Try to read message at server side. */
func (ts *testSystem) serverTryRead(size int, expectedData []byte) {
	var q struct{}
	ts.t.Logf("server starts to read...")
	_, data, err := ts.server.Read()
	if err != nil {
		ts.t.Fatalf("Server received error during read.")
		return
	}

	switch size {
	case SHORT:
		//fmt.Printf("WRONG!! Server received short message: %s\n", data)
		fmt.Printf("expected data: %s, size: %d\n", expectedData, size)
		ts.t.Fatalf("Server received short message: %s\n", data)
		return
	case LONG:
		ts.exitChan <- q
		if len(data) != len(expectedData) {
			ts.t.Fatalf("Expecting data %s, server received longer message: %s",
				expectedData, data)
		}
		return
	case NORMAL:
		ts.exitChan <- q
		if !bytes.Equal(data, expectedData) {
			ts.t.Fatalf("Expecting %s, server received message: %s",
				expectedData, data)
		}
		return
	}
}

/* Read message at server side. */
func (ts *testSystem) serverReadCorrupted(corrupted bool, testEndChan chan struct{}) {
	var q struct{}
	_, _, err := ts.server.Read()

	if !corrupted {
		ts.exitChan <- q
		if err != nil {
			ts.t.Fatalf("Server got error while reading untampered message.")
			return
		}
	} else {
		select {
		case <-testEndChan:
			return

		default:
			ts.t.Fatalf("Server failed to identify corrupted message.")
			return
		}
	}
}

/* Try to read message at client side */
func (ts *testSystem) clientTryRead(size int, expectedData []byte) {
	var q struct{}
	ts.t.Logf("client starts to read...")
	data, err := ts.clients[0].Read()
	if err != nil {
		print(err.Error())
		ts.t.Fatalf("Client received error during read.")
		return
	}

	switch size {
	case SHORT:
		print("size: ", size)
		ts.t.Fatalf("Client received short message!")
		return
	case LONG:
		ts.exitChan <- q
		if len(data) != len(expectedData) {
			ts.t.Fatalf("Expecting shorter data %s, client received longer message: %s",
				expectedData, data)
		}
		return
	case NORMAL:
		ts.exitChan <- q
		if !bytes.Equal(data, expectedData) {
			ts.t.Fatalf("Expecting %s, client received message: %s",
				expectedData, data)
		}
		return
	}
}

/* Read message at client side. */
func (ts *testSystem) clientReadCorrupted(corrupted bool, testEndChan chan struct{}) {
	var q struct{}
	_, err := ts.clients[0].Read()

	if !corrupted {
		ts.exitChan <- q
		if err != nil {
			ts.t.Fatalf("Client got error while reading untampered message.")
			return
		}
	} else {
		select {
		case <-testEndChan:
			return

		default:
			ts.t.Fatalf("Client failed to identify corrupted message.")
			return
		}
	}
}

func randData() []byte {
	// Random int r: 1000 <= r < 1,000,000
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	writeBytes, _ := json.Marshal(r.Intn(1000) * 1000)
	return writeBytes
}

func (ts *testSystem) serverSend(data []byte) {
	err := ts.server.Write(ts.clients[0].ConnID(), data)
	if err != nil {
		ts.t.Fatalf("Error returned by server.Write(): %s", err)
	}
}

func (ts *testSystem) clientSend(data []byte) {
	err := ts.clients[0].Write(data)
	if err != nil {
		ts.t.Fatalf("Error returned by client.Write(): %s", err)
	}
}

func (ts *testSystem) testServerWithVariableLengthMsg(timeout int) {
	fmt.Printf("=== %s (1 clients, 1 msgs/client, %d%% drop rate, %d window size)\n",
		ts.desc, ts.dropPercent, ts.params.WindowSize)
	data := randData()

	// First, verify that server can read normal length message
	ts.t.Logf("Testing server read with normal length data")
	go ts.serverTryRead(NORMAL, data)
	go ts.clientSend(data)

	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("Server didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}

	// Now verify that server truncates long messages
	ts.t.Logf("Testing server read with a long message")
	lspnet.SetMsgLengtheningPercent(100)
	go ts.serverTryRead(LONG, data)
	go ts.clientSend(data)

	timeoutChan = time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("Server didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}
	lspnet.SetMsgLengtheningPercent(0)

	// Last, verify that server doesn't read short messages
	ts.t.Logf("Testing the server with a short message")
	lspnet.SetMsgShorteningPercent(100)

	go ts.serverTryRead(SHORT, data)
	go ts.clientSend(data)

	// If server does receive any message before timeout, your implementation is correct
	time.Sleep(time.Duration(timeout) * time.Millisecond)
}

func (ts *testSystem) testClientWithVariableLengthMsg(timeout int) {
	fmt.Printf("=== %s (1 clients, 1 msgs/client, %d%% drop rate, %d window size)\n",
		ts.desc, ts.dropPercent, ts.params.WindowSize)
	data := randData()

	// First, verify that client can read normal length message
	ts.t.Logf("Testing client read with normal length data")
	go ts.clientTryRead(NORMAL, data)
	go ts.serverSend(data)

	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("client didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}

	// Now verify that client truncates long messages
	ts.t.Logf("Testing client read with a long message")
	lspnet.SetMsgLengtheningPercent(100)
	go ts.clientTryRead(LONG, data)
	go ts.serverSend(data)

	timeoutChan = time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("Client didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}
	lspnet.SetMsgLengtheningPercent(0)

	// Last, verify that client doesn't read short messages
	ts.t.Logf("Testing the client with a short message")
	lspnet.SetMsgShorteningPercent(100)

	go ts.clientTryRead(SHORT, data)
	go ts.serverSend(data)

	// If client does receive any message before timeout, your implementation is correct
	time.Sleep(time.Duration(timeout) * time.Millisecond)
}

func (ts *testSystem) testServerWithCorruptedMsg(timeout int) {
	fmt.Printf("=== %s (1 clients, 1 msgs/client, %d%% drop rate, %d window size)\n",
		ts.desc, ts.dropPercent, ts.params.WindowSize)

	data := randData()
	testEndChan := make(chan struct{})

	// First, verify that server can read untampered message
	ts.t.Logf("Testing server read with untampered message")
	go ts.serverReadCorrupted(false, testEndChan)
	go ts.clientSend(data)

	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("Server didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}

	// Next, verify that server can identify corrupted message
	ts.t.Logf("Testing server read with corrupted message")
	lspnet.SetMsgCorrupted(true)
	defer lspnet.SetMsgCorrupted(false)
	go ts.serverReadCorrupted(true, testEndChan)
	go ts.clientSend(data)

	// If client does receive any message before timeout, your implementation is correct
	time.Sleep(time.Duration(timeout) * time.Millisecond)
	close(testEndChan)
}

func (ts *testSystem) testClientWithCorruptedMsg(timeout int) {
	fmt.Printf("=== %s (1 clients, 1 msgs/client, %d%% drop rate, %d window size, %d max unacked messages)\n",
		ts.desc, ts.dropPercent, ts.params.WindowSize, ts.params.MaxUnackedMessages)

	data := randData()
	testEndChan := make(chan struct{})

	// First, verify that client can read untampered message
	ts.t.Logf("Testing client read with untampered message")
	go ts.clientReadCorrupted(false, testEndChan)
	go ts.serverSend(data)

	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-timeoutChan:
		ts.t.Fatalf("Client didn't receive any message in %dms", timeout)
	case <-ts.exitChan:
	}

	// Next, verify that server can identify corrupted message
	ts.t.Logf("Testing client read with corrupted message")
	lspnet.SetMsgCorrupted(true)
	defer lspnet.SetMsgCorrupted(false)
	go ts.clientReadCorrupted(true, testEndChan)
	go ts.serverSend(data)

	// If client does receive any message before timeout, your implementation is correct
	time.Sleep(time.Duration(timeout) * time.Millisecond)
	close(testEndChan)
}

func TestCorruptedMsgServer(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1, 1)).
		setDescription("TestCorruptedMsgServer: server should identify corrupted messages").
		testServerWithCorruptedMsg(2000)
}

func TestCorruptedMsgClient(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1, 1)).
		setDescription("TestCorruptedMsgClient: client should identify corrupted messages").
		testClientWithCorruptedMsg(2000)
}

func TestVariableLengthMsgServer(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1, 1)).
		setDescription("TestVariableLengthMsgServer: server should handle variable length messages").
		testServerWithVariableLengthMsg(2000)
}

func TestVariableLengthMsgClient(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1, 1)).
		setDescription("TestVariableLengthMsgClient: client should handle variable length messages").
		testClientWithVariableLengthMsg(2000)
}
