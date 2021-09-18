#!/bin/bash

#lsp1_test
go test -run=TestBasic1 -timeout=5s -race
go test -run=TestBasic2 -timeout=5s -race
go test -run=TestBasic3 -timeout=5s -race
go test -run=TestBasic4 -timeout=10s
go test -run=TestBasic5 -timeout=10s
go test -run=TestBasic6 -timeout=20s
go test -run=TestBasic7 -timeout=20s
go test -run=TestBasic8 -timeout=20s
go test -run=TestBasic9 -timeout=20s

#lsp5_test
go test -run=TestBasicISN -timeout=5s -race

#lsp2_test
go test -run=TestOutOfOrderMsg1 -timeout=10s -race
go test -run=TestOutOfOrderMsg2 -timeout=10s -race
go test -run=TestOutOfOrderMsg3 -timeout=10s -race

