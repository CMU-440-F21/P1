// LSP sliding window tests.

// TestWindow1-3 test the case that the sliding window has reached its maximum
// capacity. Specifically, they test that only 'w' unacknowledged messages
// can be sent out at once. TestWindow4-6 test the case that messages are returned
// by Read in the order they were sent (i.e. in order of their sequence numbers).
// Specifically, if messages 1-5 are dropped and messages 6-10 are received, then
// the latter 5 should not be returned by Read until the first 5 are received.

package lsp

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

type windowTestMode int

const (
	doMaxCapacity windowTestMode = iota
	doScatteredMsgs
	doOutOfWindowMsgs
	doMessageOrder
	doExponentialBackOff
)

type windowTestSystem struct {
	t                  *testing.T
	server             Server
	params             *Params
	mode               windowTestMode
	desc               string
	numClients         int
	numMsgs            int
	maxEpochs          int
	timeout            int
	clientMap          map[int]Client   // Map of connected clients.
	clientReadMsgs     map[int][]string // Map of msgs that the server reads from clients.
	serverReadMsgs     map[int][]string // Map of msgs that the clients read from server.
	serverSendMsgs     []string         // Slice of msgs that the server sends to clients.
	clientSendMsgs     []string         // Slice of msgs that clients send to the server.
	serverReadMsgsLock sync.Mutex
	clientReadMsgsLock sync.Mutex
	timeoutChan        <-chan time.Time
	exitChan           chan struct{}
	serverDoneChan     chan bool
	clientDoneChan     chan bool
}

func newWindowTestSystem(t *testing.T, mode windowTestMode, numClients, numMsgs int, params *Params) *windowTestSystem {
	ts := new(windowTestSystem)
	ts.t = t
	ts.exitChan = make(chan struct{})
	ts.clientMap = make(map[int]Client)
	ts.serverReadMsgs = make(map[int][]string)
	ts.clientReadMsgs = make(map[int][]string)
	ts.params = params
	ts.numClients = numClients
	ts.numMsgs = numMsgs
	ts.mode = mode
	ts.serverSendMsgs = ts.makeRandMsgs(numMsgs)
	ts.clientSendMsgs = ts.makeRandMsgs(numMsgs)
	ts.serverDoneChan = make(chan bool, numClients+1)
	ts.clientDoneChan = make(chan bool, numClients+1)

	// Start up the server.
	const numTries = 5
	var err error
	var port int
	for i := 0; i < numTries && ts.server == nil; i++ {
		port = 3000 + rand.Intn(50000)
		ts.server, err = NewServer(port, ts.params)
		if err != nil {
			t.Logf("Failed to start server on port %d: %s", port, err)
		}
	}
	if err != nil {
		t.Fatalf("Failed to start server.")
	}
	t.Logf("Started server on port %d.", port)

	// Start up the clients.
	hostport := lspnet.JoinHostPort("localhost", strconv.Itoa(port))
	for i := 0; i < ts.numClients; i++ {
		cli, err := NewClient(hostport, 0, ts.params)
		if err != nil {
			ts.t.Fatalf("Failed to create client: %s", err)
		}
		connID := cli.ConnID()
		ts.clientMap[connID] = cli
		ts.serverReadMsgs[connID] = nil
		ts.clientReadMsgs[connID] = nil
	}
	return ts
}

func (ts *windowTestSystem) setServerWriteDropPercent(percent int) {
	ts.t.Logf("Server write drop percent set to %d%%", percent)
	lspnet.SetServerWriteDropPercent(percent)
}

func (ts *windowTestSystem) setClientWriteDropPercent(percent int) {
	ts.t.Logf("Client write drop percent set to %d%%", percent)
	lspnet.SetClientWriteDropPercent(percent)
}

func (ts *windowTestSystem) makeRandMsgs(numMsgs int) []string {
	msgs := make([]string, numMsgs)
	for i := 0; i < numMsgs; i++ {
		msgs[i] = strconv.Itoa(rand.Int())
	}
	return msgs
}

func (ts *windowTestSystem) setDescription(desc string) *windowTestSystem {
	ts.desc = desc
	return ts
}

func (ts *windowTestSystem) setMaxEpochs(maxEpochs int) *windowTestSystem {
	ts.maxEpochs = maxEpochs
	ts.timeout = maxEpochs * ts.params.EpochMillis
	return ts
}

// Client sends a stream of messages to the server.
func (ts *windowTestSystem) streamToServer(connID int, cli Client, sendMsgs []string) {
	ts.t.Logf("Client %d streaming %d messages to the server.", connID, len(sendMsgs))
	for i, msg := range sendMsgs {
		select {
		case <-ts.exitChan:
			return
		default:
			if err := cli.Write([]byte(msg)); err != nil {
				ts.t.Errorf("Client %d failed to write message %d of %d: %s",
					connID, i+1, len(sendMsgs), err)
				ts.clientDoneChan <- false
				return
			}
		}
	}
	ts.clientDoneChan <- true
	ts.t.Logf("Client %d finished streaming %d messages to the server.", connID, len(sendMsgs))
}

// Server sends a stream of messages to a client.
func (ts *windowTestSystem) streamToClient(connID int, sendMsgs []string) {
	ts.t.Logf("Server streaming %d messages to client %d.", len(sendMsgs), connID)
	for i, msg := range sendMsgs {
		select {
		case <-ts.exitChan:
			return
		default:
			if err := ts.server.Write(connID, []byte(msg)); err != nil {
				ts.t.Errorf("Server failed to write message %d of %d: %s",
					i+1, len(sendMsgs), err)
				ts.serverDoneChan <- false
				return
			}
		}
	}
	ts.serverDoneChan <- true
	ts.t.Logf("Server finished streaming %d messages to client %d.", len(sendMsgs), connID)
}

// Client reads a stream of messages from the server.
func (ts *windowTestSystem) readFromServer(connID int, cli Client, totalMsgs int, checkpoints ...int) {
	ts.t.Logf("Client %d reading %d messages from server.", connID, ts.numMsgs)
	for i, check := 0, 0; i < totalMsgs; i++ {
		select {
		case <-ts.exitChan:
			return
		default:
			if 0 <= check && check < len(checkpoints) && i == checkpoints[check] {
				// Send a signal that the checkpoint has been reached.
				ts.clientDoneChan <- true
				check++
			}
			readBytes, err := cli.Read()
			if err != nil {
				ts.t.Errorf("Client %d failed to read message %d of %d: %s", connID, i+1, totalMsgs, err)
				ts.clientDoneChan <- false
				return
			}
			ts.t.Logf("Client %d read message %d of %d from server: %s", connID, i+1, totalMsgs, string(readBytes))
			// Get the list of messages that this client has received from the server,
			// and append the new message to the end.
			ts.clientReadMsgsLock.Lock()
			readMsgs, ok := ts.clientReadMsgs[connID]
			if !ok {
				ts.t.Errorf("Client with id %d does not exist", connID)
				ts.clientDoneChan <- false
				ts.clientReadMsgsLock.Unlock()
				return
			}
			ts.clientReadMsgs[connID] = append(readMsgs, string(readBytes))
			ts.clientReadMsgsLock.Unlock()
		}
	}
	ts.clientDoneChan <- true
	ts.t.Logf("Client %d finished reading %d messages from server.", connID, totalMsgs)
}

// Server reads a stream of messages from all currently connected clients.
func (ts *windowTestSystem) readFromAllClients(totalMsgs int, checkpoints ...int) {
	ts.t.Logf("Server reading %d total messages from clients.", totalMsgs)
	for i, check := 0, 0; i < totalMsgs; i++ {
		select {
		case <-ts.exitChan:
			return
		default:
			if 0 <= check && check < len(checkpoints) && i == checkpoints[check] {
				// Send a signal that the checkpoint has been reached.
				ts.serverDoneChan <- true
				check++
			}
			id, readBytes, err := ts.server.Read()
			if err != nil {
				ts.t.Errorf("Server failed to read message: %s", err)
				ts.serverDoneChan <- false
				return
			}
			ts.t.Logf("Server read message %d of %d from clients: %s", i+1, totalMsgs, string(readBytes))
			ts.serverReadMsgsLock.Lock()
			readMsgs, ok := ts.serverReadMsgs[id]
			if !ok {
				ts.t.Errorf("Client with id %d does not exist", id)
				ts.serverDoneChan <- false
				ts.serverReadMsgsLock.Unlock()
				return
			}
			ts.serverReadMsgs[id] = append(readMsgs, string(readBytes))
			ts.serverReadMsgsLock.Unlock()
		}
	}
	ts.serverDoneChan <- true
	ts.t.Logf("Server finished reading %d total messages from clients.", totalMsgs)
}

func (ts *windowTestSystem) waitForServer() {
	ts.t.Log("Waiting for server...")
	select {
	case <-ts.timeoutChan:
		close(ts.exitChan)
		ts.t.Fatalf("Test timed out after %.2f secs", float64(ts.timeout)/1000.0)
	case ok := <-ts.serverDoneChan:
		if !ok {
			close(ts.exitChan)
			ts.t.Fatal("Server failed due to an error.")
		}
	}
	ts.t.Log("Done waiting for server.")
}

func (ts *windowTestSystem) waitForClients() {
	ts.t.Log("Waiting for clients...")
	for i := 0; i < ts.numClients; i++ {
		select {
		case <-ts.timeoutChan:
			close(ts.exitChan)
			ts.t.Fatalf("Test timed out after %.2f secs", float64(ts.timeout)/1000.0)
		case ok := <-ts.clientDoneChan:
			if !ok {
				close(ts.exitChan)
				ts.t.Fatal("Client failed due to an error.")
			}
		}
	}
	ts.t.Log("Done waiting for clients.")
}

func (ts *windowTestSystem) checkServerReadMsgs(sentMsgs []string) {
	// Note that race conditions shouldn't occur assuming the student's
	// code is correct.
	ts.serverReadMsgsLock.Lock()
	defer ts.serverReadMsgsLock.Unlock()
	for connID, readMsgs := range ts.serverReadMsgs {
		if len(readMsgs) != len(sentMsgs) {
			ts.t.Fatalf("Server should have read %d msgs, but read %d.",
				len(sentMsgs), len(readMsgs))
		}
		for i := range sentMsgs {
			if readMsgs[i] != sentMsgs[i] {
				close(ts.exitChan)
				ts.t.Fatalf("Server received msg %s from client %d, expected %s",
					readMsgs[i], connID, sentMsgs[i])
			}
		}
	}
	ts.t.Logf("Server received expected messages from clients.")
}

func (ts *windowTestSystem) checkClientReadMsgs(sentMsgs []string) {
	// Note that race conditions shouldn't occur assuming the student's
	// code is correct.
	ts.clientReadMsgsLock.Lock()
	defer ts.clientReadMsgsLock.Unlock()
	for connID, readMsgs := range ts.clientReadMsgs {
		if len(readMsgs) != len(sentMsgs) {
			ts.t.Fatalf("Client %d should have read %d msgs, but read %d.", connID,
				len(sentMsgs), len(readMsgs))
		}
		for i := range sentMsgs {
			if readMsgs[i] != sentMsgs[i] {
				close(ts.exitChan)
				ts.t.Fatalf("Client %d received msg %s from server, expected %s",
					connID, readMsgs[i], sentMsgs[i])
			}
		}
	}
	ts.t.Logf("Clients received expected messages from server.")
}

func (ts *windowTestSystem) runTest() {
	defer lspnet.ResetDropPercent()
	fmt.Printf("=== %s (%d clients, %d msgs/client, %d window size, %d max unacked messages, %d max epochs)\n",
		ts.desc, ts.numClients, ts.numMsgs, ts.params.WindowSize, ts.params.MaxUnackedMessages, ts.maxEpochs)
	ts.timeoutChan = time.After(time.Duration(ts.timeout) * time.Millisecond)
	switch ts.mode {
	case doMaxCapacity:
		ts.runMaxCapacityTest()
	case doScatteredMsgs:
		ts.runScatteredMsgsTest()
	case doOutOfWindowMsgs:
		ts.runWindowBiggerThanUnackTest()
	case doMessageOrder:
		ts.runMessageOrderTest()
	case doExponentialBackOff:
		ts.runExponentialBackOffTest()
	}
}

func (ts *windowTestSystem) runMaxCapacityTest() {
	numClients := ts.numClients
	numMsgs := ts.numMsgs
	var msgSendSize int
	wsize := ts.params.WindowSize
	maxUnackedSize := ts.params.MaxUnackedMessages

	if wsize < maxUnackedSize {
		msgSendSize = wsize
	} else {
		msgSendSize = maxUnackedSize
	}

	if numMsgs <= msgSendSize {
		ts.t.Fatal("Number of messages must be greater than window size.")
	}

	// (1) Server to client.
	ts.setServerWriteDropPercent(100) // Don't let server send acks.

	// Tell server to begin listening for messages from clients.
	go ts.readFromAllClients(numClients*numMsgs, msgSendSize*numClients)

	for connID, cli := range ts.clientMap {
		// Instruct client to send a stream of msgs to server. The server won't
		// be able to send acks, so the client's window will fill up with
		// un-acked data msgs. Server should only read 'W' messages total, where
		// 'W' is the client's window size.
		go ts.streamToServer(connID, cli, ts.clientSendMsgs)
	}

	ts.waitForClients() // Wait for clients to finish writing messages to the server.
	ts.waitForServer()  // Wait for the server to read the messages from the clients.
	time.Sleep(50 * time.Millisecond)

	// Confirm that the server received the expected messages from the client.
	ts.checkServerReadMsgs(ts.clientSendMsgs[0:msgSendSize])

	// Let the server send acks again. Client will resume sending the
	// rest of its messages at the next epoch event.
	ts.setServerWriteDropPercent(0)

	ts.waitForServer() // Wait for the server to read the rest of the client's messages.
	time.Sleep(50 * time.Millisecond)

	// Confirm that the server received all of the expected messages from the client.
	ts.checkServerReadMsgs(ts.clientSendMsgs)

	// (2) Client to server.
	ts.setClientWriteDropPercent(100) // Don't let clients send acks.

	for connID, cli := range ts.clientMap {
		go ts.readFromServer(connID, cli, numMsgs, msgSendSize) // Tell clients to start reading from the server.
		go ts.streamToClient(connID, ts.serverSendMsgs)         // Start streaming messages to each client.
	}

	for i := 0; i < numClients; i++ {
		ts.waitForServer() // Wait for the server to finish writing messages to each client.
	}
	ts.waitForClients() // Wait for clients to read messages from the server.
	time.Sleep(50 * time.Millisecond)

	// Confirm that the client read the expected messages from the server.
	ts.checkClientReadMsgs(ts.serverSendMsgs[0:msgSendSize])

	ts.setClientWriteDropPercent(0) // Let clients send acks.

	ts.waitForClients() // Wait for client to read messages from the server.
	time.Sleep(50 * time.Millisecond)

	// Confirm that the client read the expected messages from the server.
	ts.checkClientReadMsgs(ts.serverSendMsgs)

	close(ts.exitChan)
}

func (ts *windowTestSystem) runScatteredMsgsTest() {
	numClients := ts.numClients
	numMsgs := ts.numMsgs
	wsize := ts.params.WindowSize
	maxUnackedSize := ts.params.MaxUnackedMessages
	if wsize > maxUnackedSize {
		ts.t.Fatal("Limit on unacked messages must be greater than the window size.")
	}
	if wsize <= numMsgs {
		ts.t.Fatal("Number of messages must be less than the window size.")
	}

	// (1) Client to server.
	ts.t.Logf("Testing client to server...")
	ts.setClientWriteDropPercent(100) // Don't let clients send data messages.
	for connID, cli := range ts.clientMap {
		// Drop the first half of the messages.
		go ts.streamToServer(connID, cli, ts.clientSendMsgs[0:numMsgs/2])
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.

	ts.setClientWriteDropPercent(0) // Let clients send data messages.
	for connID, cli := range ts.clientMap {
		// Successfully send the second half of the messages.
		go ts.streamToServer(connID, cli, ts.clientSendMsgs[numMsgs/2:])
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.

	// Tell server to begin listening for messages from clients and wait.
	go ts.readFromAllClients(numMsgs*numClients, 0)
	ts.waitForServer()
	time.Sleep(50 * time.Millisecond)

	// Confirm that no messages have been read yet.
	//ts.checkServerReadMsgs(ts.clientSendMsgs[0:0])

	// Wait for the server to read the rest of the client's messages.
	ts.waitForServer()
	time.Sleep(50 * time.Millisecond)

	// Confirm that the server received all of the expected messages from the client.
	ts.checkServerReadMsgs(ts.clientSendMsgs)

	// (2) Client to server.
	ts.t.Logf("Testing server to client...")
	ts.setServerWriteDropPercent(100) // Don't let the server send data messages.
	for connID := range ts.clientMap {
		// Drop the first half of the messages.
		go ts.streamToClient(connID, ts.serverSendMsgs[0:numMsgs/2])
	}
	for _ = range ts.clientMap {
		// Wait for the server to finish writing messages to a client.
		ts.waitForServer()
	}

	ts.setServerWriteDropPercent(0) // Let the server send data messages.
	for connID := range ts.clientMap {
		// Successfully send the second half of the messages.
		go ts.streamToClient(connID, ts.serverSendMsgs[numMsgs/2:])
	}
	for _ = range ts.clientMap {
		// Wait for the server to finish writing messages to a client.
		ts.waitForServer()
	}

	// Tell the clients to begin listening for messages from the server and wait.
	for connID, cli := range ts.clientMap {
		go ts.readFromServer(connID, cli, numMsgs, 0)
	}
	ts.waitForClients()
	time.Sleep(50 * time.Millisecond)

	// Confirm that no messages have been read yet.
	//ts.checkClientReadMsgs(ts.serverSendMsgs[0:0])

	// Wait for the clients to read the rest of the server's messages.
	ts.waitForClients()
	time.Sleep(50 * time.Millisecond)

	// Confirm that the clients received all of the expected messages from the server.
	ts.checkClientReadMsgs(ts.serverSendMsgs)
}

func (ts *windowTestSystem) runWindowBiggerThanUnackTest() {
	numClients := ts.numClients
	numMsgs := ts.numMsgs

	// (1) Client to server.
	ts.t.Logf("Testing client to server...")
	ts.setServerWriteDropPercent(100) // Don't let server send acks.
	for connID, cli := range ts.clientMap {
		// Send the fist 1/4th of the messages
		go ts.streamToServer(connID, cli, ts.clientSendMsgs[0:numMsgs/4])
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.

	// now we wait till expbackoff goes up so the messages are not being re-sent
	time.Sleep(2000 * time.Millisecond)

	ts.setServerWriteDropPercent(0) // Let server send back acks
	for connID, cli := range ts.clientMap {
		// send the next 1/4th of the messages
		go ts.streamToServer(connID, cli, ts.clientSendMsgs[numMsgs/4:2*numMsgs/4])
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.
	time.Sleep(200 * time.Millisecond)

	ts.setServerWriteDropPercent(100) // Don't let server send acks.
	for connID, cli := range ts.clientMap {
		// send the next 1/4th of the messages
		go ts.streamToServer(connID, cli, ts.clientSendMsgs[2*numMsgs/4:])
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.

	// Tell server to begin listening for messages from clients and wait.
	go ts.readFromAllClients(numMsgs*numClients, (3*numMsgs/4)*numClients)
	ts.waitForServer()
	// In order to test whether the window is correct, give the server enough time to
	// Read additional messages that it is not allowed to deliver until the network
	// is unblocked
	time.Sleep(500 * time.Millisecond)

	// Confirm that the server received all of the expected messages from the client.
	ts.checkServerReadMsgs(ts.clientSendMsgs[:3*numMsgs/4])

	ts.setServerWriteDropPercent(0)

	// Confirm that the remaining messages are successfully read
	// Note: if you do not consume all of the checkpoint signals before
	// the test function returns, it is a race condition
	ts.waitForServer()

	// (2) server to client
	ts.t.Logf("Testing server to client...")
	ts.setClientWriteDropPercent(100) // Don't let clients send acks.

	for connID := range ts.clientMap {
		go ts.streamToClient(connID, ts.serverSendMsgs[0:numMsgs/4]) // Start streaming messages to each client.
	}

	for i := 0; i < numClients; i++ {
		ts.waitForServer() // Wait for the server to finish writing messages to each client.
	}

	time.Sleep(2000 * time.Millisecond)

	ts.setClientWriteDropPercent(0) // Let client send back acks
	for connID := range ts.clientMap {
		go ts.streamToClient(connID, ts.serverSendMsgs[numMsgs/4:2*numMsgs/4]) // Start streaming messages to each client.
	}
	for i := 0; i < numClients; i++ {
		ts.waitForServer() // Wait for the server to finish writing messages to each client.
	}
	time.Sleep(200 * time.Millisecond)

	ts.setClientWriteDropPercent(100) // Don't let clients send back acks
	for connID := range ts.clientMap {
		go ts.streamToClient(connID, ts.serverSendMsgs[2*numMsgs/4:]) // Start streaming messages to each client.
	}
	time.Sleep(200 * time.Millisecond)

	for i := 0; i < numClients; i++ {
		ts.waitForServer() // Wait for the server to finish writing messages to each client.
	}

	for connID, cli := range ts.clientMap {
		go ts.readFromServer(connID, cli, numMsgs, 3*numMsgs/4) // Tell clients to start reading from the server.
	}

	ts.waitForClients() // Wait for clients to read messages from the server.
	// In order to test whether the window is correct, give the client enough time to
	// Read additional messages that it is not allowed to deliver until the network is unblocked
	time.Sleep(500 * time.Millisecond)

	// Confirm that the server received all of the expected messages from the client.
	ts.checkClientReadMsgs(ts.serverSendMsgs[:3*numMsgs/4])

	ts.setClientWriteDropPercent(0) // Let client send back acks

	// Confirm that the remaining messages are successfully read by each client
	ts.waitForClients() // Wait for clients to read all messages from the server.
}

func (ts *windowTestSystem) runMessageOrderTest() {
	numClients := ts.numClients
	numMsgs := ts.numMsgs
	wsize := ts.params.WindowSize
	if wsize <= numMsgs {
		ts.t.Fatal("Number of messages must be less than the window size.")
	}

	ts.t.Logf("Testing client to server...")
	for connID, cli := range ts.clientMap {
		go ts.streamToServer(connID, cli, ts.clientSendMsgs)
	}
	ts.waitForClients() // Wait for clients to finish writing messages to the server.

	// Tell server to begin listening for messages from clients and wait.
	go ts.readFromAllClients(numMsgs*numClients, numMsgs*numClients)
	ts.waitForServer()

	// Confirm that the server received all of the expected messages from the client.
	ts.checkServerReadMsgs(ts.clientSendMsgs)
}

// 12 epochs for 5 data: XX.X..X....X
// 2 epochs for grace time to compensate any time skew
const ExponentialBackOffTestEpochToListen = 14

func (ts *windowTestSystem) runExponentialBackOffTest() {
	numClients := ts.numClients
	numMsgs := ts.numMsgs
	wsize := ts.params.WindowSize
	epochLen := ts.params.EpochMillis
	maxBackoff := ts.params.MaxBackOffInterval
	if maxBackoff < 4 {
		ts.t.Fatal("MaxBackOffInterval must be greater than 3.")
	}
	if wsize >= numMsgs {
		ts.t.Fatal("Number of messages must be greater than the window size.")
	}

	ts.t.Logf("Testing client to server...")
	ts.setServerWriteDropPercent(100) // Don't let server send acks.
	lspnet.StartSniff()
	for connID, cli := range ts.clientMap {
		go ts.streamToServer(connID, cli, ts.clientSendMsgs)
	}
	time.Sleep(time.Millisecond * time.Duration(epochLen*ExponentialBackOffTestEpochToListen))
	var sniffRes = lspnet.StopSniff()
	if sniffRes.NumSentData > wsize*numClients*6 || sniffRes.NumSentData < wsize*numClients*4 {
		ts.t.Log(sniffRes)
		ts.t.Fatalf("Number of trying messages does not match: expected [%d, %d], got %d",
			wsize*numClients*4, wsize*numClients*6, sniffRes.NumSentData)
	}
	fmt.Println(sniffRes.NumSentData)
	ts.setServerWriteDropPercent(0)
}

func TestExpBackOff1(t *testing.T) {
	newWindowTestSystem(t, doExponentialBackOff, 1, 10, &Params{100, 2000, 5, 4, 5}).
		setDescription("TestExpBackOff1: 1 clients, backoff test").
		setMaxEpochs(ExponentialBackOffTestEpochToListen + 5).
		runTest()
}

func TestExpBackOff2(t *testing.T) {
	newWindowTestSystem(t, doExponentialBackOff, 10, 15, &Params{100, 2000, 5, 4, 5}).
		setDescription("TestExpBackOff2: 10 clients, backoff test").
		setMaxEpochs(ExponentialBackOffTestEpochToListen + 5).
		runTest()
}

func TestWindow1(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 1, 10, &Params{3, 500, 5, 0, 50}).
		setDescription("TestWindow1: 1 client, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestWindow2(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 5, 25, &Params{3, 500, 10, 0, 50}).
		setDescription("TestWindow2: 5 clients, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestWindow3(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 10, 25, &Params{3, 500, 10, 0, 50}).
		setDescription("TestWindow3: 10 clients, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestWindow4(t *testing.T) {
	newWindowTestSystem(t, doScatteredMsgs, 1, 10, &Params{3, 1000, 20, 0, 20}).
		setDescription("TestWindow4: 1 client, scattered msgs").
		setMaxEpochs(5).
		runTest()
}

func TestWindow5(t *testing.T) {
	newWindowTestSystem(t, doScatteredMsgs, 5, 10, &Params{3, 1000, 20, 0, 20}).
		setDescription("TestWindow5: 5 clients, scattered msgs").
		setMaxEpochs(5).
		runTest()
}

func TestWindow6(t *testing.T) {
	newWindowTestSystem(t, doScatteredMsgs, 10, 10, &Params{3, 1000, 20, 0, 20}).
		setDescription("TestWindow6: 10 clients, scattered msgs").
		setMaxEpochs(5).
		runTest()
}

func TestMaxUnackedMessages1(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 1, 10, &Params{3, 500, 50, 0, 5}).
		setDescription("TestMaxUnackedMessages1: 1 client, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestMaxUnackedMessages2(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 5, 25, &Params{3, 500, 50, 0, 10}).
		setDescription("TestMaxUnackedMessages2: 5 clients, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestMaxUnackedMessages3(t *testing.T) {
	newWindowTestSystem(t, doMaxCapacity, 10, 25, &Params{3, 500, 50, 0, 10}).
		setDescription("TestMaxUnackedMessages3: 10 clients, max capacity").
		setMaxEpochs(5).
		runTest()
}

func TestMaxUnackedMessages4(t *testing.T) {
	newWindowTestSystem(t, doOutOfWindowMsgs, 1, 20, &Params{100, 1000, 20, 10, 10}).
		setDescription("TestMaxUnackedMessages4: 1 client, window and max unacked msgs").
		setMaxEpochs(10).
		runTest()
}

func TestMaxUnackedMessages5(t *testing.T) {
	newWindowTestSystem(t, doOutOfWindowMsgs, 5, 20, &Params{100, 1000, 15, 10, 10}).
		setDescription("TestMaxUnackedMessages5: 5 clients, window and max unacked msgs").
		setMaxEpochs(10).
		runTest()
}

func TestMaxUnackedMessages6(t *testing.T) {
	newWindowTestSystem(t, doOutOfWindowMsgs, 5, 20, &Params{100, 1000, 20, 10, 10}).
		setDescription("TestMaxUnackedMessages6: 5 clients, window and max unacked msgs").
		setMaxEpochs(10).
		runTest()
}

func TestOutOfOrderMsg1(t *testing.T) {
	lspnet.SetDelayMessagePercent(50)
	defer lspnet.SetDelayMessagePercent(0)
	newWindowTestSystem(t, doMessageOrder, 1, 10, &Params{3, 5000, 30, 0, 30}).
		setDescription("TestOutOfOrderMsg1: 1 client, out-of-order test").
		setMaxEpochs(5).
		runTest()
}

func TestOutOfOrderMsg2(t *testing.T) {
	lspnet.SetDelayMessagePercent(50)
	defer lspnet.SetDelayMessagePercent(0)
	newWindowTestSystem(t, doMessageOrder, 5, 25, &Params{3, 5000, 30, 0, 30}).
		setDescription("TestOutOfOrderMsg2: 5 clients, out-of-order test").
		setMaxEpochs(5).
		runTest()
}

func TestOutOfOrderMsg3(t *testing.T) {
	lspnet.SetDelayMessagePercent(50)
	defer lspnet.SetDelayMessagePercent(0)
	newWindowTestSystem(t, doMessageOrder, 10, 25, &Params{3, 5000, 30, 0, 30}).
		setDescription("TestOutOfOrderMsg3: 10 clients, out-of-order test").
		setMaxEpochs(5).
		runTest()
}
