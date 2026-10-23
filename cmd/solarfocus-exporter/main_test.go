package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slauger/solarfocus-exporter/internal/solarfocus"
)

func TestParseList(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{"", nil, false},
		{"1", []int{1}, false},
		{"1, 2,3", []int{1, 2, 3}, false},
		{"1,,2", []int{1, 2}, false},
		{"a", nil, true},
	}
	for _, tt := range tests {
		got, err := parseList(tt.in)
		if (err != nil) != tt.wantErr || !slices.Equal(got, tt.want) {
			t.Errorf("parseList(%q) = %v, %v; want %v, err %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("SOLARFOCUS_TEST_SET", "x")
	if got := envOr("SOLARFOCUS_TEST_SET", "d"); got != "x" {
		t.Errorf("got %q, want x", got)
	}
	if got := envOr("SOLARFOCUS_TEST_UNSET", "d"); got != "d" {
		t.Errorf("got %q, want d", got)
	}
}

func TestRunInvalidArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing address", nil},
		{"bad unit id", []string{"-modbus.address", "x:502", "-modbus.unit-id", "300"}},
		{"bad circuits", []string{"-modbus.address", "x:502", "-heating-circuits", "a"}},
		{"circuit out of range", []string{"-modbus.address", "x:502", "-heating-circuits", "9"}},
		{"bad tanks", []string{"-modbus.address", "x:502", "-hot-water-tanks", "x"}},
		{"bad buffers", []string{"-modbus.address", "x:502", "-buffers", "x"}},
		{"unknown flag", []string{"-nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOLARFOCUS_MODBUS_ADDRESS", "")
			if err := run(tt.args); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	if err := run([]string{"-version"}); err != nil {
		t.Fatal(err)
	}
}

// stubReader returns zero for every register.
type stubReader struct{}

func (stubReader) ReadInputRegisters(_ context.Context, _, quantity uint16) ([]uint16, error) {
	return make([]uint16, quantity), nil
}

func (stubReader) ReadHoldingRegisters(_ context.Context, _, quantity uint16) ([]uint16, error) {
	return make([]uint16, quantity), nil
}

func TestHandler(t *testing.T) {
	cfg := solarfocus.Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}, Timeout: time.Second}
	srv := httptest.NewServer(newHandler(stubReader{}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)

	for path, want := range map[string]string{
		"/metrics": "solarfocus_up 1",
		"/healthz": "ok",
		"/":        "/metrics",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Errorf("%s: status %d, body does not contain %q", path, resp.StatusCode, want)
		}
	}
}
