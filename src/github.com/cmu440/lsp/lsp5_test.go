// LSP message tests

// These tests ensure that the client and server are interacting
// using the correct sequence number progression (following ISN).

package lsp

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

type msgTestSystem struct {
	t              *testing.T
	server         Server
	params         *Params
	desc           string
	serverPort     int
	numClients     int
	randGenerator  *rand.Rand
	clients        map[int]Client // Map of connected clients.
	clientISNs     map[int]int    // Map of ISNs chosen for clients.
	exitChan       chan struct{}
	serverDoneChan chan bool
	clientDoneChan chan bool
}

func newMsgTestSystem(t *testing.T, numClients int, params *Params) *msgTestSystem {
	ts := new(msgTestSystem)
	ts.t = t
	ts.exitChan = make(chan struct{})
	ts.clientISNs = make(map[int]int)
	ts.clients = make(map[int]Client)
	ts.params = params
	ts.serverPort = 0
	ts.randGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
	ts.numClients = numClients
	ts.serverDoneChan = make(chan bool, numClients+1)
	ts.clientDoneChan = make(chan bool, numClients+1)

	return ts
}

func (ts *msgTestSystem) setDescription(t string) *msgTestSystem {
	ts.desc = t
	return ts
}

func (ts *msgTestSystem) startServer() {
	// Start up the server.
	const numTries = 5
	var err error
	var port int
	for i := 0; i < numTries && ts.server == nil; i++ {
		port = 3000 + rand.Intn(50000)
		ts.server, err = NewServer(port, ts.params)
		if err != nil {
			ts.t.Logf("Failed to start server on port %d: %s", port, err)
		}
	}
	if err != nil {
		ts.t.Fatalf("Failed to start server.")
	}
	ts.t.Logf("Started server on port %d.", port)
	ts.serverPort = port
}

func (ts *msgTestSystem) startClients() {
	// Start up the clients with random ISNs
	hostport := lspnet.JoinHostPort("localhost", strconv.Itoa(ts.serverPort))
	for i := 0; i < ts.numClients; i++ {
		isn := ts.randGenerator.Intn(int(math.Pow(2, 8))) + 1
		cli, err := NewClient(hostport, isn, ts.params)
		if err != nil {
			lspnet.StopSniff()
			ts.t.Fatalf("Failed to create client: %s", err)
		}
		id := cli.ConnID()
		ts.clients[id] = cli
		ts.clientISNs[id] = isn
	}
}

// Runs the client, sending numWrites messages to the server
func (ts *msgTestSystem) runWriteClient(clientID, numWrites int, doneChan chan<- bool) {
	writeMsg := []byte("TestMessage")
	cli := ts.clients[clientID]
	connID := cli.ConnID()
	for i := 0; i < numWrites; i++ {
		select {
		case <-ts.exitChan:
			return
		default:
			err := cli.Write(writeMsg)
			if err != nil {
				ts.t.Errorf("Client %d write got error: %s.", connID, err)
				doneChan <- false
				return
			}
		}
	}
	doneChan <- true
}

// An "ACK coalescing" middlebox. Replaces sequences of Acks it
// sees in a connection with a single CAck. Since we can't tell
// the direction of communication from the message alone, it's
// important that only one connection end-point actively sends
// data; the other must exclusively send ACKs.
type ACKCoalescerMiddlebox struct {
	t               *testing.T  // Pointer to test instance
	randGenerator   *rand.Rand  // PRNG
	numConnections  int         // Total Number of connections
	randomizeDrops  bool        // Whether to randomize ACK drops
	lastExpectedSNs map[int]int // Maps Conn IDs to last expected ACK SNs
}

func (m *ACKCoalescerMiddlebox) Run(
	msg *lspnet.TemporaryMessage) lspnet.MiddleboxOutput {
	var output lspnet.MiddleboxOutput
	output.ModifiedMsg = false
	output.SendMsg = true

	// Ignore anything that is a non-data Ack
	if msg.Type == lspnet.TypeMsgAck && msg.SeqNum != 0 {
		// Fetch the last expected message SN for this flow
		if lastSN, ok := m.lastExpectedSNs[msg.ConnID]; ok {
			if msg.SeqNum < lastSN {
				if !m.randomizeDrops || (m.randGenerator.Intn(2) == 1) {
					// Drop this ACK
					output.SendMsg = false
					m.t.Logf("Dropped ACK with SeqNum %d for ConnID %d\n",
						msg.SeqNum, msg.ConnID)
				}
			} else if msg.SeqNum == lastSN {
				// Mutate final ACK into a CAck. TODO:
				// Currently assumes zero loss. Update
				// this to handle OOO sequence numbers.
				msg.Type = lspnet.TypeMsgCAck
				output.ModifiedMsg = true

				m.t.Logf("Replaced an Ack with SeqNum %d for ConnID %d by a CAck\n",
					msg.SeqNum, msg.ConnID)
			} else {
				m.t.Fatalf("Unexpected SeqNum %d for ConnID %d",
					msg.SeqNum, msg.ConnID)
			}
		} else {
			m.t.Fatalf("Invalid ConnID %d.", msg.ConnID)
		}
	}
	return output
}

// Fetch the message sequence for each connection
func (ts *msgTestSystem) parseMessages(
	sniffRes *lspnet.SniffResult) map[int][]*lspnet.TemporaryMessage {
	msgs := make(map[int][]*lspnet.TemporaryMessage)
	for _, msg := range sniffRes.SentMessages {
		msgs[msg.ConnID] = append(msgs[msg.ConnID], msg)
	}
	return msgs
}

func (ts *msgTestSystem) runReadServer() {
	defer ts.t.Log("Server starting...")
	for {
		select {
		case <-ts.exitChan:
			return
		default:
			_, _, err := ts.server.Read()
			if err != nil {
				ts.t.Logf("Server received error during read.")
				return
			}
		}
	}
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func (ts *msgTestSystem) runBasicISNTest(timeout int) {
	fmt.Printf("=== %s (%d clients, %d window size, %d max unacked messages)\n",
		ts.desc, ts.numClients, ts.params.WindowSize, ts.params.MaxUnackedMessages)

	ts.startServer() // Start the server
	defer close(ts.exitChan)

	lspnet.StartSniff() // Start sniffing
	ts.startClients()   // Start clients

	go ts.runReadServer()
	for i := range ts.clients {
		go ts.runWriteClient(i, 1, ts.clientDoneChan)
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	for range ts.clients {
		select {
		case <-timeoutChan:
			lspnet.StopSniff()
			ts.t.Fatalf("Test timed out after %.2f secs", float64(timeout)/1000.0)
		case ok := <-ts.clientDoneChan:
			if !ok {
				lspnet.StopSniff()
				ts.t.Fatalf("Client failed due to an error.")
			}
		}
	}
	// Give a reasonable amount of time for message exchange to complete
	time.Sleep(time.Duration(ts.params.EpochMillis) * time.Millisecond)
	sniffResult := lspnet.StopSniff()
	msgs := ts.parseMessages(&sniffResult)

	// Ensure the ISN progression is correct. We expect
	// to see a total of 3 messages for each connection
	// (excluding the original Connect message, which
	// will have a connection ID of 0).
	for cli, connectionMsgs := range msgs {
		if isn, ok := ts.clientISNs[cli]; ok {
			expectedSNProgression := []int{
				isn, isn + 1, isn + 1,
			}
			eSNIdx, msgIdx := 0, 0
			for msgIdx < len(connectionMsgs) {
				// Ignore Epoch ACKs, we may receive an additional
				// (ACK, id, 0) from the client's epoch msg.
				if connectionMsgs[msgIdx].Type == int(MsgAck) &&
					connectionMsgs[msgIdx].SeqNum == 0 {
					msgIdx++

				} else if eSNIdx < len(expectedSNProgression) {
					if expectedSNProgression[eSNIdx] != connectionMsgs[msgIdx].SeqNum {
						ts.t.Fatalf("Unexpected SeqNum progression.")
					}
					msgIdx++
					eSNIdx++

				} else {
					break
				}
			}
			// Saw too few messages
			if eSNIdx != len(expectedSNProgression) {
				ts.t.Fatalf("Saw too few messages for ConnId %d (expected at least %d)",
					cli, len(expectedSNProgression))
			}
		} else if cli != 0 {
			ts.t.Fatalf("Invalid ConnID %d.", cli)
		}
	}
	// Ensure that we saw a message exchange for every connection
	if len(msgs) != (len(ts.clientISNs) + 1) {
		ts.t.Fatalf("Expected to see %d connections, saw %d.",
			len(ts.clientISNs), max(0, len(msgs)-1))
	}
}

func (ts *msgTestSystem) runCAckTestServer(
	randomizeNumWrites, randomizeAckDrops bool, maxEpochs, timeout int) {
	fmt.Printf("=== %s (%d clients, %d window size, %d max unacked messages)\n",
		ts.desc, ts.numClients, ts.params.WindowSize, ts.params.MaxUnackedMessages)

	ts.startServer()  // Start the server
	ts.startClients() // Start clients
	defer close(ts.exitChan)

	// Initialize the middlebox
	m := &ACKCoalescerMiddlebox{
		t:               ts.t,
		numConnections:  ts.numClients,
		randGenerator:   ts.randGenerator,
		randomizeDrops:  randomizeAckDrops,
		lastExpectedSNs: make(map[int]int),
	}
	numWrites := ts.params.MaxUnackedMessages
	if randomizeNumWrites {
		numWrites = ts.randGenerator.Intn(ts.params.MaxUnackedMessages) + 1
	}

	for id := range ts.clients {
		m.lastExpectedSNs[id] = ts.clientISNs[id] + numWrites
	}
	lspnet.StartMiddlebox(m)

	go ts.runReadServer()
	for i := range ts.clients {
		go ts.runWriteClient(i, numWrites, ts.clientDoneChan)
	}
	// Give a reasonable amount of time for clients to complete writes
	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	for range ts.clients {
		select {
		case <-timeoutChan:
			lspnet.StopMiddlebox()
			ts.t.Fatalf("Test timed out after %.2f secs", float64(timeout)/1000.0)
		case ok := <-ts.clientDoneChan:
			if !ok {
				lspnet.StopMiddlebox()
				ts.t.Fatalf("Client failed due to an error.")
			}
		}
	}
	// Give a reasonable amount of time for the CAcks to propagate and be processed
	time.Sleep(time.Duration(2*ts.params.EpochMillis) * time.Millisecond)
	lspnet.StopMiddlebox()

	// By this point, all clients should know that they're
	// done. Start the sniffer and listen for maxNumEpochs
	// to see if any data messages are being retransmitted
	// by clients; if so, signal failure.
	lspnet.StartSniff()
	time.Sleep(time.Duration(maxEpochs*ts.params.EpochMillis) * time.Millisecond)
	sniffResult := lspnet.StopSniff()

	if sniffResult.NumSentData != 0 {
		ts.t.Fatalf("One or more data messages sent after everything was CAck'd!")
	}
}

func TestBasicISN(t *testing.T) {
	newMsgTestSystem(t, 100, makeParams(5, 2000, 1, 1)).
		setDescription("TestBasicISN: Ensure the ISN progression is correct").
		runBasicISNTest(1000)
}

func TestCAckServer1(t *testing.T) {
	newMsgTestSystem(t, 5, makeParams(5, 1000, 1, 1)).
		setDescription("TestCAckServer1: Replaces a single Ack with CAck.").
		runCAckTestServer(false, false, 12, 1000)
}

func TestCAckServer2(t *testing.T) {
	newMsgTestSystem(t, 5, makeParams(5, 1000, 100, 50)).
		setDescription("TestCAckServer2: Replaces 50 Acks with one CAck.").
		runCAckTestServer(false, false, 12, 1000)
}

func TestCAckServer3(t *testing.T) {
	newMsgTestSystem(t, 5, makeParams(5, 1000, 100, 100)).
		setDescription("TestCAckServer3: Replaces a random number of Acks with one CAck.").
		runCAckTestServer(true, false, 12, 1000)
}

func TestCAckServer4(t *testing.T) {
	newMsgTestSystem(t, 5, makeParams(5, 1000, 100, 100)).
		setDescription("TestCAckServer4: Randomly sends/drops ACKs, followed by one CAck.").
		runCAckTestServer(true, true, 12, 1000)
}
