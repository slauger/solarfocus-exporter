// Package modbus implements the small subset of Modbus TCP needed by the exporter:
// reading input registers (function code 0x04) and holding registers (0x03).
package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	fcReadHoldingRegisters = 0x03
	fcReadInputRegisters   = 0x04

	mbapHeaderLen = 7
	// Modbus allows at most 125 registers per read request.
	maxRegisters = 125
)

// ExceptionError is returned when the device answers with a Modbus exception.
type ExceptionError struct {
	FunctionCode byte
	Code         byte
}

func (e *ExceptionError) Error() string {
	return fmt.Sprintf("modbus exception %d for function code 0x%02x", e.Code, e.FunctionCode)
}

// Client is a Modbus TCP client that keeps one connection open and serializes requests.
type Client struct {
	address string
	unitID  byte
	timeout time.Duration

	mu    sync.Mutex
	conn  net.Conn
	txnID uint16
}

// NewClient returns a client for host:port. No connection is opened until the first request.
func NewClient(address string, unitID byte, timeout time.Duration) *Client {
	return &Client{address: address, unitID: unitID, timeout: timeout}
}

// ReadInputRegisters reads quantity input registers starting at address.
func (c *Client) ReadInputRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error) {
	return c.read(ctx, fcReadInputRegisters, address, quantity)
}

// ReadHoldingRegisters reads quantity holding registers starting at address.
func (c *Client) ReadHoldingRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error) {
	return c.read(ctx, fcReadHoldingRegisters, address, quantity)
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) read(ctx context.Context, fc byte, address, quantity uint16) ([]uint16, error) {
	if quantity == 0 || quantity > maxRegisters {
		return nil, fmt.Errorf("invalid register quantity %d", quantity)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	regs, err := c.requestLocked(ctx, fc, address, quantity)
	if err == nil {
		return regs, nil
	}
	if _, ok := errors.AsType[*ExceptionError](err); ok || ctx.Err() != nil {
		return nil, err
	}
	// The device may have closed an idle connection: reconnect once and retry.
	_ = c.closeLocked()
	return c.requestLocked(ctx, fc, address, quantity)
}

func (c *Client) requestLocked(ctx context.Context, fc byte, address, quantity uint16) ([]uint16, error) {
	if c.conn == nil {
		d := net.Dialer{Timeout: c.timeout}
		conn, err := d.DialContext(ctx, "tcp", c.address)
		if err != nil {
			return nil, fmt.Errorf("connect %s: %w", c.address, err)
		}
		c.conn = conn
	}

	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		_ = c.closeLocked()
		return nil, err
	}

	c.txnID++
	req := encodeRequest(c.txnID, c.unitID, fc, address, quantity)
	if _, err := c.conn.Write(req); err != nil {
		_ = c.closeLocked()
		return nil, fmt.Errorf("write request: %w", err)
	}

	regs, err := readResponse(c.conn, c.txnID, fc, quantity)
	if err != nil {
		if _, ok := errors.AsType[*ExceptionError](err); !ok {
			_ = c.closeLocked()
		}
		return nil, err
	}
	return regs, nil
}

func encodeRequest(txnID uint16, unitID, fc byte, address, quantity uint16) []byte {
	b := make([]byte, mbapHeaderLen+5)
	binary.BigEndian.PutUint16(b[0:], txnID)
	binary.BigEndian.PutUint16(b[2:], 0) // protocol identifier
	binary.BigEndian.PutUint16(b[4:], 6) // unit id + PDU
	b[6] = unitID
	b[7] = fc
	binary.BigEndian.PutUint16(b[8:], address)
	binary.BigEndian.PutUint16(b[10:], quantity)
	return b
}

func readResponse(r io.Reader, txnID uint16, fc byte, quantity uint16) ([]uint16, error) {
	header := make([]byte, mbapHeaderLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if got := binary.BigEndian.Uint16(header[0:]); got != txnID {
		return nil, fmt.Errorf("transaction id mismatch: got %d, want %d", got, txnID)
	}
	length := int(binary.BigEndian.Uint16(header[4:]))
	if length < 2 || length > 256 {
		return nil, fmt.Errorf("invalid response length %d", length)
	}
	pdu := make([]byte, length-1) // length includes the unit id
	if _, err := io.ReadFull(r, pdu); err != nil {
		return nil, fmt.Errorf("read pdu: %w", err)
	}

	switch {
	case pdu[0] == fc|0x80:
		if len(pdu) < 2 {
			return nil, errors.New("short exception response")
		}
		return nil, &ExceptionError{FunctionCode: fc, Code: pdu[1]}
	case pdu[0] != fc:
		return nil, fmt.Errorf("unexpected function code 0x%02x", pdu[0])
	}

	if len(pdu) < 2 || int(pdu[1]) != int(quantity)*2 || len(pdu) != 2+int(quantity)*2 {
		return nil, fmt.Errorf("unexpected byte count in response for %d registers", quantity)
	}
	regs := make([]uint16, quantity)
	for i := range regs {
		regs[i] = binary.BigEndian.Uint16(pdu[2+2*i:])
	}
	return regs, nil
}
