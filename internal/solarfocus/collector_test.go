package solarfocus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeReader serves registers from maps; missing addresses return an error.
type fakeReader struct {
	input   map[uint16]uint16
	holding map[uint16]uint16
}

func (f *fakeReader) ReadInputRegisters(_ context.Context, address, quantity uint16) ([]uint16, error) {
	return read(f.input, address, quantity)
}

func (f *fakeReader) ReadHoldingRegisters(_ context.Context, address, quantity uint16) ([]uint16, error) {
	return read(f.holding, address, quantity)
}

func read(m map[uint16]uint16, address, quantity uint16) ([]uint16, error) {
	out := make([]uint16, quantity)
	for i := range quantity {
		v, ok := m[address+i]
		if !ok {
			return nil, errors.New("illegal data address")
		}
		out[i] = v
	}
	return out, nil
}

func neg(v int16) uint16 { return uint16(v) }

// installation mirrors a real pellet boiler with one heating circuit and one hot water tank.
func installation() *fakeReader {
	return &fakeReader{
		input: map[uint16]uint16{
			// boiler 2400-2409: 54.2 °C, alarm, message 14, door closed, cleaning n/a, ash 27 %, outdoor 8.8 °C
			2400: 542, 2401: 40, 2402: 0, 2403: 0, 2404: 14, 2405: 0, 2406: neg(-1), 2407: 27, 2408: 88, 2409: 0,
			// heating circuit 1: flow 71.7, no room sensor (121.8), humidity n/a, pump off, heating
			1100: 717, 1101: 1218, 1102: neg(-1), 1103: 1, 1104: 0, 1105: 1, 1106: 0, 1107: 2,
			// hot water 1: 68.7 °C, ready, always on
			500: 687, 501: 1, 502: 1,
			// buffer 1: sensors not connected
			1900: 1300, 1901: 1300, 1902: neg(-999), 1903: 0, 1904: 0, 1905: 1,
		},
		holding: map[uint16]uint16{
			32602: 0, 32603: 2,
			32000: 600, 32001: 0,
		},
	}
}

func newTestCollector(r RegisterReader, cfg Config) *Collector {
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second
	}
	return NewCollector(r, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCollect(t *testing.T) {
	c := newTestCollector(installation(), Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}, Buffers: []int{1}})

	want := `
# HELP solarfocus_boiler_ash_box_percent Ash box fill level, should be emptied at 100.
# TYPE solarfocus_boiler_ash_box_percent gauge
solarfocus_boiler_ash_box_percent 27
# HELP solarfocus_boiler_message_number Number of the active message on the controller, 0 if none.
# TYPE solarfocus_boiler_message_number gauge
solarfocus_boiler_message_number 14
# HELP solarfocus_boiler_state_info Boiler status as text, value is always 1.
# TYPE solarfocus_boiler_state_info gauge
solarfocus_boiler_state_info{code="40",state="Alarm aktiv!"} 1
# HELP solarfocus_boiler_temperature_celsius Boiler (Kessel) temperature.
# TYPE solarfocus_boiler_temperature_celsius gauge
solarfocus_boiler_temperature_celsius 54.2
# HELP solarfocus_heating_circuit_mode_info Heating circuit operating mode as text, value is always 1.
# TYPE solarfocus_heating_circuit_mode_info gauge
solarfocus_heating_circuit_mode_info{circuit="1",code="2",mode="Automatik"} 1
# HELP solarfocus_heating_circuit_pump_on 1 if the heating circuit pump is running.
# TYPE solarfocus_heating_circuit_pump_on gauge
solarfocus_heating_circuit_pump_on{circuit="1"} 0
# HELP solarfocus_hot_water_target_temperature_celsius Hot water tank target temperature (holding register).
# TYPE solarfocus_hot_water_target_temperature_celsius gauge
solarfocus_hot_water_target_temperature_celsius{tank="1"} 60
# HELP solarfocus_outdoor_temperature_celsius Outdoor temperature.
# TYPE solarfocus_outdoor_temperature_celsius gauge
solarfocus_outdoor_temperature_celsius 8.8
# HELP solarfocus_up 1 if all configured registers were read successfully.
# TYPE solarfocus_up gauge
solarfocus_up 1
`
	names := []string{
		"solarfocus_boiler_ash_box_percent", "solarfocus_boiler_message_number", "solarfocus_boiler_state_info",
		"solarfocus_boiler_temperature_celsius", "solarfocus_heating_circuit_mode_info", "solarfocus_heating_circuit_pump_on",
		"solarfocus_hot_water_target_temperature_celsius", "solarfocus_outdoor_temperature_celsius", "solarfocus_up",
	}
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), names...); err != nil {
		t.Fatal(err)
	}
}

func TestMissingSensorsAreSkipped(t *testing.T) {
	c := newTestCollector(installation(), Config{HeatingCircuits: []int{1}, Buffers: []int{1}})

	for _, name := range []string{
		"solarfocus_heating_circuit_room_temperature_celsius", // 121.8 °C = no room sensor
		"solarfocus_heating_circuit_humidity_percent",         // -1
		"solarfocus_boiler_cleaning_percent",                  // -1
		"solarfocus_buffer_temperature_celsius",               // 130 °C and -999
	} {
		if n := testutil.CollectAndCount(c, name); n != 0 {
			t.Errorf("%s: got %d series, want 0", name, n)
		}
	}
	if n := testutil.CollectAndCount(c, "solarfocus_heating_circuit_flow_temperature_celsius"); n != 1 {
		t.Errorf("flow temperature: got %d series, want 1", n)
	}
}

func TestReadErrorSetsUpToZero(t *testing.T) {
	r := installation()
	delete(r.input, 501) // hot water block fails
	c := newTestCollector(r, Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}})

	want := `
# HELP solarfocus_up 1 if all configured registers were read successfully.
# TYPE solarfocus_up gauge
solarfocus_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), "solarfocus_up"); err != nil {
		t.Fatal(err)
	}
	// Other parts are still exported.
	if n := testutil.CollectAndCount(c, "solarfocus_boiler_temperature_celsius"); n != 1 {
		t.Errorf("boiler temperature: got %d series, want 1", n)
	}
}

func TestLint(t *testing.T) {
	c := newTestCollector(installation(), Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}, Buffers: []int{1}})
	problems, err := testutil.CollectAndLint(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}

func TestDescribe(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(newTestCollector(installation(), Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}, Buffers: []int{1}}))
	if _, err := reg.Gather(); err != nil {
		t.Fatal(err)
	}
}

func TestStateText(t *testing.T) {
	tests := []struct {
		table map[int]string
		code  int
		want  string
	}{
		{boilerStates, 2, "Pelletsbetrieb"},
		{boilerStates, 40, "Alarm aktiv!"},
		{boilerStates, 43, "code 43"},
		{boilerStates, 202, "code 202"},
		{heatingCircuitStates, 7, "Heizkreis nicht freigeschaltet"},
		{dhwStates, 2, "Laden"},
		{bufferStates, 2, "Puffer wird beladen"},
	}
	for _, tt := range tests {
		if got := stateText(tt.table, tt.code); got != tt.want {
			t.Errorf("stateText(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"defaults", Config{HeatingCircuits: []int{1}, HotWaterTanks: []int{1}}, false},
		{"all", Config{HeatingCircuits: []int{1, 8}, HotWaterTanks: []int{4}, Buffers: []int{1, 4}}, false},
		{"circuit 9", Config{HeatingCircuits: []int{9}}, true},
		{"tank 0", Config{HotWaterTanks: []int{0}}, true},
		{"buffer 5", Config{Buffers: []int{5}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateConfig(tt.cfg); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
