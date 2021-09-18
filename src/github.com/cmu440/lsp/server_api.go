// DO NOT MODIFY THIS FILE!

package lsp

// Server defines the interface for a LSP server.
type Server interface {
	// Read reads a data message from a client and returns its payload,
	// and the connection ID associated with the client that sent the message.
	// This method should block until data has been received from some client.
	// It should return a non-nil error if either (1) the connection to some
	// client has been explicitly closed, (2) the connection to some client
	// has been lost due to an epoch timeout and no other messages from that
	// client are waiting to be returned, or (3) Close() has been called on the server.
	// In the first two cases, the client's connection ID and a non-nil
	// error should be returned. In the third case, an ID with value 0 and
	// a non-nil error should be returned. Note that in the third case,
	// it is also ok for Read to never return anything.
	Read() (int, []byte, error)

	// Write sends a data message to the client with the specified connection ID.
	// This method should NOT block, and should return a non-nil error if the
	// connection with the client has been lost. If Close has been called on the server,
	// subsequent calls to Write must either return a non-nil error, or never return anything.
	Write(connID int, payload []byte) error

	// CloseConn terminates the client with the specified connection ID, returning
	// a non-nil error if the specified connection ID does not exist. All pending
	// messages to the client should be sent and acknowledged. However, unlike Close,
	// this method should NOT block.
	CloseConn(connID int) error

	// Close terminates all currently connected clients and shuts down the LSP server.
	// This method should block until all pending messages for each client are sent
	// and acknowledged. If one or more clients are lost during this time, a non-nil
	// error should be returned. Once it returns, all goroutines running in the
	// background should exit.
	//
	// Note that after Close is called, further calls to Read, Write, CloseConn, and Close
	// must either return a non-nil error, or never return anything.
	Close() error
}
