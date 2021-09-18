// LSP server/client close tests.

// These tests check that the client/server Close methods work correctly.
// Pending messages should be returned by Read and pending messages should
// be written and acknowledged by the other side before Close returns (note
// that CloseConn behaves slightly differently---instead of blocking it should
// return immediately). These tests also test that a client is able to connect
// to a slow-starting server (i.e. if the server starts a few epochs later than
// a client, the presence of epoch events should ensure that the connection is
// eventually established).

package lsp

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

type closeTestMode int

const (
	doSlowStart closeTestMode = iota
	doClientClose
	doServerCloseConns
	doServerClose
)

const (
	defaultNumMsgs     = 10
	defaultDelayEpochs = 3
)

type clientTermStatus struct {
	ok bool
	id int
}

type closeTestSystem struct {
	t              *testing.T
	server         Server
	clients        []Client
	params         *Params
	mode           closeTestMode
	port           int
	serverDoneChan chan bool
	clientDoneChan chan clientTermStatus
	exitChan       chan struct{}
	numClients     int
	numMsgs        int
	desc           string
	maxEpochs      int
	delayEpochs    int
}

func newCloseTestSystem(t *testing.T, mode closeTestMode) *closeTestSystem {
	ts := new(closeTestSystem)
	ts.t = t
	ts.mode = mode
	ts.clientDoneChan = make(chan clientTermStatus)
	ts.serverDoneChan = make(chan bool)
	ts.exitChan = make(chan struct{})
	ts.port = 2000 + rand.Intn(50000)
	ts.numMsgs = defaultNumMsgs
	ts.delayEpochs = defaultDelayEpochs
	return ts
}

func (ts *closeTestSystem) setNumClients(numClients int) *closeTestSystem {
	ts.clients = make([]Client, numClients)
	ts.numClients = numClients
	return ts
}

func (ts *closeTestSystem) setDescription(t string) *closeTestSystem {
	ts.desc = t
	return ts
}

func (ts *closeTestSystem) setMaxEpochs(maxEpochs int) *closeTestSystem {
	ts.maxEpochs = maxEpochs
	return ts
}

func (ts *closeTestSystem) setParams(epochLimit, epochMillis, windowSize, maxUnackedMessages int) *closeTestSystem {
	ts.params = &Params{
		EpochLimit:         epochLimit,
		EpochMillis:        epochMillis,
		WindowSize:         windowSize,
		MaxUnackedMessages: maxUnackedMessages,
		MaxBackOffInterval: 0,
	}
	return ts
}

func (ts *closeTestSystem) runTest() {
	t := ts.t
	fmt.Printf("=== %s (%d clients, %d msgs/client, %d max epochs, %d window size, %d max unacked messages)\n",
		ts.desc, ts.numClients, ts.numMsgs, ts.maxEpochs, ts.params.WindowSize, ts.params.MaxUnackedMessages)
	go ts.buildServer()
	for i := range ts.clients {
		go ts.buildClient(i)
	}

	millis := ts.maxEpochs * ts.params.EpochMillis
	timeoutChan := time.After(time.Duration(millis) * time.Millisecond)

	switch ts.mode {
	case doClientClose:
		// Wait for the server.
		select {
		case ok := <-ts.serverDoneChan:
			if !ok {
				close(ts.exitChan)
				t.Fatalf("Server error")
			}
		case <-timeoutChan:
			close(ts.exitChan)
			t.Fatalf("Test timed out waiting for server")
		}
		t.Log("Server done!")
		// Wait for the clients.
		for _ = range ts.clients {
			select {
			case termStatus := <-ts.clientDoneChan:
				if !termStatus.ok {
					close(ts.exitChan)
					t.Fatalf("Client failed due to an error.")
				}
			case <-timeoutChan:
				close(ts.exitChan)
				t.Fatalf("Test timed out waiting for client")
			}
		}
		t.Log("Clients done!")
	default:
		// Wait for the clients.
		for range ts.clients {
			select {
			case termStatus := <-ts.clientDoneChan:
				if !termStatus.ok {
					close(ts.exitChan)
					t.Fatalf("Client error")
				}
			case <-timeoutChan:
				close(ts.exitChan)
				t.Fatalf("Test timed out waiting for client")
			}
		}
		// Wait for the server.
		select {
		case ok := <-ts.serverDoneChan:
			if !ok {
				close(ts.exitChan)
				t.Fatalf("Server error")
			}
		case <-timeoutChan:
			close(ts.exitChan)
			t.Fatalf("Test timed out waiting for server")
		}
	}
	close(ts.exitChan)
}

func (ts *closeTestSystem) createServer() error {
	srv, err := NewServer(ts.port, ts.params)
	ts.server = srv
	return err
}

func (ts *closeTestSystem) createClient(index int) error {
	cli, err := NewClient(lspnet.JoinHostPort("localhost", strconv.Itoa(ts.port)), 0, ts.params)
	ts.clients[index] = cli
	return err
}

func (ts *closeTestSystem) buildServer() {
	t := ts.t
	if ts.mode == doSlowStart {
		// Delay the server's start to let the clients start first.
		t.Logf("Delaying server start by %d epochs", ts.delayEpochs)
		time.Sleep(time.Duration(ts.delayEpochs*ts.params.EpochMillis) * time.Millisecond)
	}

	// Create/start the Server.
	if ts.createServer() != nil {
		t.Errorf("Couldn't create server on port %d", ts.port)
		ts.serverDoneChan <- false
		return
	}

	var numDead, numEcho int
	for {
		select {
		case <-ts.exitChan:
			t.Log("Received exit signal in buildServer. Shutting down...")
			return
		default:
			id, data, err := ts.server.Read()
			if err != nil {
				// If an error occurred, then assume a client has died.
				t.Logf("Server detected dead connection (client %d)", id)
				numDead++
				if ts.mode == doClientClose && numDead == ts.numClients {
					t.Logf("Detected termination of all %d clients", numDead)
					ts.server.Close()
					ts.serverDoneChan <- true
					rwOpsStopped := ts.testServerClose()
					if !rwOpsStopped {
						t.Fatalf("Server error: Read or Write allowed after close")
					}
					return
				}
			} else {
				// Otherwise, echo the message back to the client immediately.
				err := ts.server.Write(id, data)
				if err != nil {
					t.Errorf("Server failed to write to client connection %d", id)
					ts.serverDoneChan <- false
					return
				}
				numEcho++
				if numEcho == ts.numClients*ts.numMsgs {
					t.Logf("Echoed all %d * %d messsages", ts.numClients, ts.numMsgs)
					if ts.mode == doServerClose {
						t.Log("Server closing all client connections...")
						ts.server.Close()
						ts.serverDoneChan <- true
						rwOpsStopped := ts.testServerClose()
						if !rwOpsStopped {
							t.Fatalf("Server error: Read or Write allowed after close")
						}
						return
					}
					if ts.mode == doServerCloseConns {
						for _, cli := range ts.clients {
							ts.server.CloseConn(cli.ConnID())
						}
						ts.serverDoneChan <- true
						return
					}
					if ts.mode != doClientClose {
						ts.serverDoneChan <- true
						ts.server.CloseConn(id)
						return
					}
				}
			}
		}
	}
}

// Create client. Have it generate messages and check that they are echoed.
func (ts *closeTestSystem) buildClient(clientID int) {
	t := ts.t
	if ts.createClient(clientID) != nil {
		t.Errorf("Failed to create client %d on port %d", clientID, ts.port)
		ts.clientDoneChan <- clientTermStatus{false, clientID}
		return
	}
	cli := ts.clients[clientID]
	var mcount int
	for mcount = 0; mcount < ts.numMsgs; mcount++ {
		select {
		case <-ts.exitChan:
			t.Log("Received exit signal in buildClient. Shutting down...")
			return
		default:
			tv := mcount*100 + rand.Intn(100)
			b, _ := json.Marshal(tv)
			err := cli.Write(b)
			if err != nil {
				t.Errorf("Failed write by client %d", clientID)
				ts.clientDoneChan <- clientTermStatus{false, clientID}
				return
			}
			b, berr := cli.Read()
			if berr != nil {
				t.Errorf("Client %d detected server termination after only %d messages",
					clientID, mcount)
				ts.clientDoneChan <- clientTermStatus{false, clientID}
				cli.Close()
				return
			}
			var v int
			json.Unmarshal(b, &v)
			if v != tv {
				t.Errorf("Client %d got %d. Expected %d\n", clientID, v, tv)
				ts.clientDoneChan <- clientTermStatus{false, clientID}
				cli.Close()
				return
			}
		}
	}

	t.Logf("Client %d completed %d messages", clientID, ts.numMsgs)
	switch {
	case ts.mode == doClientClose:
		t.Logf("Closing client %d...", clientID)
		cli.Close()
		ts.clientDoneChan <- clientTermStatus{true, clientID}
		rwOpsStopped := ts.testClientClose(clientID)
		if !rwOpsStopped {
			t.Fatalf("Client error: Read or Write allowed after close")
		}
	case ts.mode == doServerCloseConns || ts.mode == doServerClose:
		t.Logf("Client %d attempting read", clientID)
		_, err := cli.Read()
		if err != nil {
			t.Logf("Client %d detected server termination after %d messages",
				clientID, mcount)
			ts.clientDoneChan <- clientTermStatus{true, clientID}
			cli.Close()
			rwOpsStopped := ts.testClientClose(clientID)
			if !rwOpsStopped {
				t.Fatalf("Client error: Read or Write allowed after close")
			}
			return
		}
		t.Errorf("Client %d received unexpected data from server", clientID)
		ts.clientDoneChan <- clientTermStatus{false, clientID}
		cli.Close()
	default:
		// Note that we do not defer closing of the client (since we are testing
		// whether the students implemented close correctly in some of the tests).
		cli.Close()
		ts.clientDoneChan <- clientTermStatus{true, clientID}
		rwOpsStopped := ts.testClientClose(clientID)
		if !rwOpsStopped {
			t.Fatalf("Client error: Read or Write allowed after close")
		}

	}
}

func (ts *closeTestSystem) testServerClose() bool {

	t := ts.t

	//Test that read returns with error immediately
	serverReadChan := make(chan error)
	go func(serverReadChan chan error) {
		_, _, err := ts.server.Read()
		serverReadChan <- err
	}(serverReadChan)

	millis := ts.params.EpochMillis
	timeoutChan := time.After(time.Duration(millis) * time.Millisecond)
	select {
	case <-timeoutChan:
	case err := <-serverReadChan:
		if err == nil {
			t.Logf("Server closed, should not allow Reads")
			return false
		}
	}

	//Test that write returns with error, note we assume write never blocks
	for _, cli := range ts.clients {
		connID := cli.ConnID()
		tv := connID*100 + rand.Intn(100)
		data, _ := json.Marshal(tv)
		err := ts.server.Write(connID, data)
		if err == nil {
			t.Logf("Server closed, should not allow Writes")
			return false
		}
	}

	return true
}

func (ts *closeTestSystem) testClientClose(clientID int) bool {
	t := ts.t

	cli := ts.clients[clientID]

	//Test that read returns with error immediately
	clientReadChan := make(chan error)
	go func(clientReadChan chan error) {
		_, err := cli.Read()
		clientReadChan <- err
	}(clientReadChan)

	millis := ts.params.EpochMillis
	timeoutChan := time.After(time.Duration(millis) * time.Millisecond)

	select {
	case <-timeoutChan:
	case err := <-clientReadChan:
		if err == nil {
			t.Logf("Client %d closed, should not allow Reads!", cli.ConnID())
			return false
		}
	}

	//Test that write returns with error, note we assume write never blocks
	tv := cli.ConnID()*100 + rand.Intn(100)
	data, _ := json.Marshal(tv)
	err := cli.Write(data)
	if err == nil {
		t.Logf("Client %d closed, should not allow Writes!", cli.ConnID())
		return false
	}

	return true
}

func TestServerSlowStart1(t *testing.T) {
	newCloseTestSystem(t, doSlowStart).
		setDescription("TestServerSlowStart1: Delayed server start").
		setNumClients(1).
		setMaxEpochs(5).
		setParams(5, 500, 1, 1).
		runTest()
}

func TestServerSlowStart2(t *testing.T) {
	newCloseTestSystem(t, doSlowStart).
		setDescription("TestServerSlowStart2: Delayed server start").
		setNumClients(3).
		setMaxEpochs(5).
		setParams(5, 500, 1, 1).
		runTest()
}

func TestServerClose1(t *testing.T) {
	newCloseTestSystem(t, doServerClose).
		setDescription("TestServerClose1: Server Close correctness").
		setNumClients(1).
		setMaxEpochs(10).
		setParams(5, 500, 1, 1).
		runTest()
}

func TestServerClose2(t *testing.T) {
	newCloseTestSystem(t, doServerClose).
		setDescription("TestServerClose2: Server Close correctness").
		setNumClients(3).
		setMaxEpochs(5).
		setParams(2, 500, 1, 1).
		runTest()
}

func TestServerCloseConns1(t *testing.T) {
	newCloseTestSystem(t, doServerCloseConns).
		setDescription("TestServerCloseConns1: Server CloseConns correctness").
		setNumClients(1).
		setMaxEpochs(10).
		setParams(5, 500, 1, 1).
		runTest()
}

func TestServerCloseConns2(t *testing.T) {
	newCloseTestSystem(t, doServerCloseConns).
		setDescription("TestServerCloseConns2: Server CloseConns correctness").
		setNumClients(3).
		setMaxEpochs(5).
		setParams(2, 500, 1, 1).
		runTest()
}

func TestClientClose1(t *testing.T) {
	newCloseTestSystem(t, doClientClose).
		setDescription("TestClientClose1: Client Close correctness").
		setNumClients(2).
		setMaxEpochs(10).
		setParams(5, 500, 1, 1).
		runTest()
}

func TestClientClose2(t *testing.T) {
	newCloseTestSystem(t, doClientClose).
		setDescription("TestClientClose2: Client Close correctness").
		setNumClients(3).
		setMaxEpochs(15).
		setParams(5, 500, 1, 1).
		runTest()
}
