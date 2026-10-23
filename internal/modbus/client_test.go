package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// fakeServer answers Modbus TCP read requests with register value = address,
// or with an exception for address 9999.
func fakeServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn)
		}
	}()
	return ln.Addr().String()
}

func serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		req := make([]byte, 12)
		if _, err := io.ReadFull(conn, req); err != nil {
			return
		}
		fc := req[7]
		addr := binary.BigEndian.Uint16(req[8:])
		qty := binary.BigEndian.Uint16(req[10:])

		var pdu []byte
		if addr == 9999 {
			pdu = []byte{fc | 0x80, 0x02}
		} else {
			pdu = []byte{fc, byte(qty * 2)}
			for i := range qty {
				pdu = binary.BigEndian.AppendUint16(pdu, addr+i)
			}
		}
		resp := make([]byte, 7, 7+len(pdu))
		copy(resp, req[:4])
		binary.BigEndian.PutUint16(resp[4:], uint16(len(pdu)+1))
		resp[6] = req[6]
		resp = append(resp, pdu...)
		if _, err := conn.Write(resp); err != nil {
			return
		}
	}
}

func TestReadRegisters(t *testing.T) {
	c := NewClient(fakeServer(t), 1, time.Second)
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()

	tests := []struct {
		name string
		read func(context.Context, uint16, uint16) ([]uint16, error)
		addr uint16
		qty  uint16
	}{
		{"input", c.ReadInputRegisters, 2400, 10},
		{"holding", c.ReadHoldingRegisters, 32600, 8},
		{"single", c.ReadInputRegisters, 500, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regs, err := tt.read(ctx, tt.addr, tt.qty)
			if err != nil {
				t.Fatal(err)
			}
			if len(regs) != int(tt.qty) {
				t.Fatalf("got %d registers, want %d", len(regs), tt.qty)
			}
			for i, v := range regs {
				if v != tt.addr+uint16(i) {
					t.Errorf("register %d = %d, want %d", i, v, tt.addr+uint16(i))
				}
			}
		})
	}
}

func TestException(t *testing.T) {
	c := NewClient(fakeServer(t), 1, time.Second)
	t.Cleanup(func() { _ = c.Close() })

	_, err := c.ReadInputRegisters(context.Background(), 9999, 1)
	var exc *ExceptionError
	if !errors.As(err, &exc) || exc.Code != 2 {
		t.Fatalf("expected exception 2, got %v", err)
	}
	// The connection must still be usable after an exception.
	if _, err := c.ReadInputRegisters(context.Background(), 1, 1); err != nil {
		t.Fatalf("read after exception: %v", err)
	}
}

func TestInvalidQuantity(t *testing.T) {
	c := NewClient("127.0.0.1:1", 1, time.Second)
	for _, q := range []uint16{0, 126} {
		if _, err := c.ReadInputRegisters(context.Background(), 0, q); err == nil {
			t.Errorf("quantity %d: expected error", q)
		}
	}
}

func TestConnectError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c := NewClient(addr, 1, 200*time.Millisecond)
	if _, err := c.ReadInputRegisters(context.Background(), 0, 1); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestEncodeRequest(t *testing.T) {
	got := encodeRequest(7, 1, fcReadInputRegisters, 2400, 10)
	want := []byte{0, 7, 0, 0, 0, 6, 1, 0x04, 0x09, 0x60, 0, 10}
	if !bytes.Equal(got, want) {
		t.Errorf("got % x, want % x", got, want)
	}
}

func TestReadResponseErrors(t *testing.T) {
	tests := []struct {
		name string
		resp []byte
	}{
		{"short header", []byte{0, 1, 0}},
		{"wrong txn", []byte{0, 2, 0, 0, 0, 5, 1, 4, 2, 0, 1}},
		{"wrong fc", []byte{0, 1, 0, 0, 0, 5, 1, 3, 2, 0, 1}},
		{"wrong count", []byte{0, 1, 0, 0, 0, 5, 1, 4, 4, 0, 1}},
		{"bad length", []byte{0, 1, 0, 0, 0, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := readResponse(bytes.NewReader(tt.resp), 1, fcReadInputRegisters, 1); err == nil {
				t.Error("expected error")
			}
		})
	}
}
