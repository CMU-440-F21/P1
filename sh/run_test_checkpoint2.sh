#!/bin/bash
go test -run=TestBasic1 -timeout=5s -race
go test -run=TestBasic2 -timeout=5s -race
go test -run=TestBasic3 -timeout=5s -race
go test -run=TestBasic4 -timeout=10s
go test -run=TestBasic5 -timeout=10s
go test -run=TestBasic6 -timeout=20s
go test -run=TestBasic7 -timeout=20s
go test -run=TestBasic8 -timeout=20s
go test -run=TestBasic9 -timeout=20s
go test -run=TestBasicISN -timeout=5s -race
go test -run=TestSendReceive1 -timeout=5s -race
go test -run=TestSendReceive2 -timeout=5s
go test -run=TestSendReceive3 -timeout=10s
go test -run=TestRobust1 -timeout=20s -race
go test -run=TestRobust2 -timeout=20s
go test -run=TestRobust3 -timeout=20s
go test -run=TestRobust4 -timeout=20s
go test -run=TestRobust5 -timeout=20s -race
go test -run=TestRobust6 -timeout=20s -race
go test -run=TestWindow1 -timeout=5s
go test -run=TestWindow2 -timeout=5s
go test -run=TestWindow3 -timeout=5s
go test -run=TestWindow4 -timeout=10s
go test -run=TestWindow5 -timeout=10s -race
go test -run=TestWindow6 -timeout=10s -race
go test -run=TestOutOfOrderMsg1 -timeout=10s -race
go test -run=TestOutOfOrderMsg2 -timeout=10s -race
go test -run=TestOutOfOrderMsg3 -timeout=10s -race
go test -run=TestExpBackOff1 -timeout=60s -race
go test -run=TestExpBackOff2 -timeout=60s -race
go test -run=TestMaxUnackedMessages1 -timeout=60s -race
go test -run=TestMaxUnackedMessages2 -timeout=60s -race
go test -run=TestMaxUnackedMessages3 -timeout=60s -race
go test -run=TestMaxUnackedMessages4 -timeout=60s -race
go test -run=TestMaxUnackedMessages5 -timeout=60s -race
go test -run=TestMaxUnackedMessages6 -timeout=60s -race
go test -run=TestServerSlowStart1 -timeout=5s -race
go test -run=TestServerSlowStart2 -timeout=5s -race
go test -run=TestServerClose1 -timeout=10s -race
go test -run=TestServerClose2 -timeout=10s -race
go test -run=TestServerCloseConns1 -timeout=10s -race
go test -run=TestServerCloseConns2 -timeout=10s -race
go test -run=TestClientClose1 -timeout=20s -race
go test -run=TestClientClose2 -timeout=20s -race
go test -run=TestServerFastClose1 -timeout=20s -race
go test -run=TestServerFastClose2 -timeout=20s
go test -run=TestServerFastClose3 -timeout=20s
go test -run=TestServerToClient1 -timeout=20s -race
go test -run=TestServerToClient2 -timeout=20s
go test -run=TestServerToClient3 -timeout=20s
go test -run=TestClientToServer1 -timeout=20s -race
go test -run=TestClientToServer2 -timeout=20s
go test -run=TestClientToServer3 -timeout=20s
go test -run=TestRoundTrip1 -timeout=20s -race
go test -run=TestRoundTrip2 -timeout=20s
go test -run=TestRoundTrip3 -timeout=30s
go test -run=TestVariableLengthMsgServer -timeout=3s -race
go test -run=TestVariableLengthMsgClient -timeout=3s
go test -run=TestCorruptedMsgServer -timeout=3s -race
go test -run=TestCorruptedMsgClient -timeout=3s
go test -run=TestCAckServer1 -timeout=20s -race
go test -run=TestCAckServer2 -timeout=20s -race
go test -run=TestCAckServer3 -timeout=20s -race
go test -run=TestCAckServer4 -timeout=20s -race
