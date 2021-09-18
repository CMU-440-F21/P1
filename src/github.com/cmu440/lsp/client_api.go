// DO NOT MODIFY THIS FILE!

package lsp

// Client defines the interface for a LSP client.
type Client interface {
	// ConnID returns the connection ID associated with this client.
	ConnID() int

	// Read reads a data message from the server and returns its payload.
	// This method should block until data has been received from the server and
	// is ready to be returned. It should return a non-nil error if either
	// (1) the connection has been explicitly closed, (2) the connection has
	// been lost due to an epoch timeout and no other messages are waiting to be
	// returned, or (3) the server is closed. Note that in the third case, it is
	// also ok for Read to never return anything.
	Read() ([]byte, error)

	// Write sends a data message with the specified payload to the server.
	// This method should NOT block, and should return a non-nil error
	// if the connection with the server has been lost. If Close has been called on
	// the client, subsequent calls to Write must either return a non-nil error, or
	// never return anything.
	Write(payload []byte) error

	// Close terminates the client's connection with the server. It should block
	// until all pending messages to the server have been sent and acknowledged.
	// Once it returns, all goroutines running in the background should exit.
	//
	// Note that after Close is called, further calls to Read, Write, and Close
	// must either return a non-nil error, or never return anything.
	Close() error
}
