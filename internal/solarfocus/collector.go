// Package solarfocus exposes the Modbus TCP registers of a SOLARFOCUS ecomanager-touch
// controller as Prometheus metrics.
package solarfocus

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "solarfocus"

// RegisterReader reads Modbus registers; implemented by modbus.Client.
type RegisterReader interface {
	ReadInputRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error)
	ReadHoldingRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error)
}

// Config selects which parts of the installation are read.
type Config struct {
	// HeatingCircuits are the circuit numbers (1-8) to read.
	HeatingCircuits []int
	// HotWaterTanks are the domestic hot water tank ("Boiler") numbers (1-4) to read.
	HotWaterTanks []int
	// Buffers are the buffer tank numbers (1-4) to read.
	Buffers []int
	// Timeout for one complete scrape.
	Timeout time.Duration
}

// Collector implements prometheus.Collector. Registers are read on every scrape.
type Collector struct {
	reader RegisterReader
	cfg    Config
	logger *slog.Logger
	mu     sync.Mutex // one scrape at a time, the controller is slow
}

// NewCollector returns a collector reading from r.
func NewCollector(r RegisterReader, cfg Config, logger *slog.Logger) *Collector {
	return &Collector{reader: r, cfg: cfg, logger: logger}
}

func desc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, "", name), help, labels, nil)
}

var (
	descUp             = desc("up", "1 if all configured registers were read successfully.")
	descScrapeDuration = desc("scrape_duration_seconds", "Time it took to read all registers.")

	descBoilerTemp       = desc("boiler_temperature_celsius", "Boiler (Kessel) temperature.")
	descBoilerState      = desc("boiler_state", "Boiler status code (input register 2401).")
	descBoilerStateInfo  = desc("boiler_state_info", "Boiler status as text, value is always 1.", "code", "state")
	descBoilerMessage    = desc("boiler_message_number", "Number of the active message on the controller, 0 if none.")
	descBoilerDoorOpen   = desc("boiler_door_open", "1 if the boiler door is open.")
	descBoilerCleaning   = desc("boiler_cleaning_percent", "Boiler cleaning indicator, cleaning recommended at 100.")
	descBoilerAshBox     = desc("boiler_ash_box_percent", "Ash box fill level, should be emptied at 100.")
	descBoilerFuelType   = desc("boiler_fuel_type", "Fuel/operating type (therminator only, register 2409).")
	descOutdoorTemp      = desc("outdoor_temperature_celsius", "Outdoor temperature.")
	descCircuitFlow      = desc("heating_circuit_flow_temperature_celsius", "Heating circuit flow temperature.", "circuit")
	descCircuitRoom      = desc("heating_circuit_room_temperature_celsius", "Room temperature (if a room sensor is installed).", "circuit")
	descCircuitHumidity  = desc("heating_circuit_humidity_percent", "Room humidity (if a humidity sensor is installed).", "circuit")
	descCircuitLimit     = desc("heating_circuit_limit_thermostat_closed", "1 if the limit thermostat is closed (ok).", "circuit")
	descCircuitPump      = desc("heating_circuit_pump_on", "1 if the heating circuit pump is running.", "circuit")
	descCircuitMixer     = desc("heating_circuit_mixer_percent", "Mixer position.", "circuit")
	descCircuitState     = desc("heating_circuit_state", "Heating circuit status code.", "circuit")
	descCircuitStateInfo = desc("heating_circuit_state_info", "Heating circuit status as text, value is always 1.", "circuit", "code", "state")
	descCircuitCooling   = desc("heating_circuit_cooling", "1 if the circuit is set to cooling.", "circuit")
	descCircuitMode      = desc("heating_circuit_mode", "Heating circuit operating mode code.", "circuit")
	descCircuitModeInfo  = desc("heating_circuit_mode_info", "Heating circuit operating mode as text, value is always 1.", "circuit", "code", "mode")
	descHWTemp           = desc("hot_water_temperature_celsius", "Domestic hot water tank (Boiler) temperature.", "tank")
	descHWState          = desc("hot_water_state", "Hot water tank status code.", "tank")
	descHWStateInfo      = desc("hot_water_state_info", "Hot water tank status as text, value is always 1.", "tank", "code", "state")
	descHWRelease        = desc("hot_water_release_mode", "Hot water tank release mode code.", "tank")
	descHWReleaseInfo    = desc("hot_water_release_mode_info", "Hot water tank release mode as text, value is always 1.", "tank", "code", "mode")
	descHWTarget         = desc("hot_water_target_temperature_celsius", "Hot water tank target temperature (holding register).", "tank")
	descHWSingleCharge   = desc("hot_water_single_charge_active", "1 if a single charge is active.", "tank")
	descBufferTemp       = desc("buffer_temperature_celsius", "Buffer tank temperature.", "buffer", "sensor")
	descBufferCharging   = desc("buffer_charging", "1 if the buffer tank is being charged.", "buffer")
	descBufferState      = desc("buffer_state", "Buffer tank status code.", "buffer")
	descBufferStateInfo  = desc("buffer_state_info", "Buffer tank status as text, value is always 1.", "buffer", "code", "state")
	descBufferRelease    = desc("buffer_release_mode", "Buffer tank release mode code.", "buffer")
)

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		descUp, descScrapeDuration, descBoilerTemp, descBoilerState, descBoilerStateInfo,
		descBoilerMessage, descBoilerDoorOpen, descBoilerCleaning, descBoilerAshBox,
		descBoilerFuelType, descOutdoorTemp, descCircuitFlow, descCircuitRoom, descCircuitHumidity,
		descCircuitLimit, descCircuitPump, descCircuitMixer, descCircuitState, descCircuitStateInfo,
		descCircuitCooling, descCircuitMode, descCircuitModeInfo,
		descHWTemp, descHWState, descHWStateInfo, descHWRelease, descHWReleaseInfo, descHWTarget,
		descHWSingleCharge, descBufferTemp, descBufferCharging, descBufferState, descBufferStateInfo,
		descBufferRelease,
	} {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.Timeout)
	defer cancel()

	start := time.Now()
	s := &scrape{ch: ch, ok: true, logger: c.logger}
	s.boiler(ctx, c.reader)
	for _, n := range c.cfg.HeatingCircuits {
		s.heatingCircuit(ctx, c.reader, n)
	}
	for _, n := range c.cfg.HotWaterTanks {
		s.hotWater(ctx, c.reader, n)
	}
	for _, n := range c.cfg.Buffers {
		s.buffer(ctx, c.reader, n)
	}

	ch <- prometheus.MustNewConstMetric(descUp, prometheus.GaugeValue, boolValue(s.ok))
	ch <- prometheus.MustNewConstMetric(descScrapeDuration, prometheus.GaugeValue, time.Since(start).Seconds())
}

// scrape holds the state of one Collect call.
type scrape struct {
	ch     chan<- prometheus.Metric
	ok     bool
	logger *slog.Logger
}

func (s *scrape) read(ctx context.Context, holding bool, address, quantity uint16, r RegisterReader) []uint16 {
	if !s.ok && ctx.Err() != nil {
		return nil
	}
	var regs []uint16
	var err error
	if holding {
		regs, err = r.ReadHoldingRegisters(ctx, address, quantity)
	} else {
		regs, err = r.ReadInputRegisters(ctx, address, quantity)
	}
	if err != nil {
		s.ok = false
		s.logger.Warn("reading registers failed", "address", address, "quantity", quantity, "holding", holding, "err", err)
		return nil
	}
	return regs
}

func (s *scrape) gauge(d *prometheus.Desc, v float64, labels ...string) {
	s.ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
}

// temperature emits a temperature register (value * 10) unless it signals a missing or
// defective sensor. The controller reports e.g. 1300 (130 °C) for an unconnected
// outdoor sensor, 2700 for a short circuit and -999 for an unused buffer sensor.
func (s *scrape) temperature(d *prometheus.Desc, raw uint16, labels ...string) {
	v := signed(raw)
	if v <= -500 || v >= 1200 {
		return
	}
	s.gauge(d, float64(v)/10, labels...)
}

// percent emits a percentage register unless it is out of range (-1 = not available).
func (s *scrape) percent(d *prometheus.Desc, raw uint16, labels ...string) {
	v := signed(raw)
	if v < 0 || v > 100 {
		return
	}
	s.gauge(d, float64(v), labels...)
}

func (s *scrape) state(code, info *prometheus.Desc, table map[int]string, raw uint16, labels ...string) {
	v := int(raw)
	s.gauge(code, float64(v), labels...)
	s.gauge(info, 1, append(labels, strconv.Itoa(v), stateText(table, v))...)
}

func (s *scrape) boiler(ctx context.Context, r RegisterReader) {
	regs := s.read(ctx, false, 2400, 10, r)
	if regs == nil {
		return
	}
	s.temperature(descBoilerTemp, regs[0])
	s.state(descBoilerState, descBoilerStateInfo, boilerStates, regs[1])
	// regs[2], regs[3]: 2402/2403 are not documented
	s.gauge(descBoilerMessage, float64(signed(regs[4])))
	s.gauge(descBoilerDoorOpen, float64(regs[5]&1))
	s.percent(descBoilerCleaning, regs[6])
	s.percent(descBoilerAshBox, regs[7])
	s.temperature(descOutdoorTemp, regs[8])
	s.gauge(descBoilerFuelType, float64(signed(regs[9])))
}

func (s *scrape) heatingCircuit(ctx context.Context, r RegisterReader, n int) {
	label := strconv.Itoa(n)
	offset := uint16(n-1) * 50 //nolint:gosec // n is validated to be 1-8

	if regs := s.read(ctx, false, 1100+offset, 8, r); regs != nil {
		s.temperature(descCircuitFlow, regs[0], label)
		s.temperature(descCircuitRoom, regs[1], label)
		s.percent(descCircuitHumidity, regs[2], label)
		s.gauge(descCircuitLimit, float64(regs[3]&1), label)
		// 1105: 0 = on, 1 = off
		s.gauge(descCircuitPump, boolValue(regs[5] == 0), label)
		s.percent(descCircuitMixer, regs[6], label)
		s.state(descCircuitState, descCircuitStateInfo, heatingCircuitStates, regs[7], label)
	}
	// Holding registers 32600/32605 (flow/room target) are values written via Modbus and
	// read back as 0 when unused; 32601/32604 are not mapped (exception 2). Only cooling
	// and the operating mode are read.
	if regs := s.read(ctx, true, 32602+offset, 2, r); regs != nil {
		s.gauge(descCircuitCooling, float64(regs[0]&1), label)
		s.state(descCircuitMode, descCircuitModeInfo, heatingCircuitModes, regs[1], label)
	}
}

func (s *scrape) hotWater(ctx context.Context, r RegisterReader, n int) {
	label := strconv.Itoa(n)
	offset := uint16(n-1) * 50 //nolint:gosec // n is validated to be 1-4

	if regs := s.read(ctx, false, 500+offset, 3, r); regs != nil {
		s.temperature(descHWTemp, regs[0], label)
		s.state(descHWState, descHWStateInfo, dhwStates, regs[1], label)
		s.state(descHWRelease, descHWReleaseInfo, releaseModes, regs[2], label)
	}
	if regs := s.read(ctx, true, 32000+offset, 2, r); regs != nil {
		s.temperature(descHWTarget, regs[0], label)
		s.gauge(descHWSingleCharge, float64(regs[1]&1), label)
	}
}

func (s *scrape) buffer(ctx context.Context, r RegisterReader, n int) {
	label := strconv.Itoa(n)
	offset := uint16(n-1) * 20 //nolint:gosec // n is validated to be 1-4

	regs := s.read(ctx, false, 1900+offset, 6, r)
	if regs == nil {
		return
	}
	s.temperature(descBufferTemp, regs[0], label, "top")
	s.temperature(descBufferTemp, regs[1], label, "bottom")
	s.temperature(descBufferTemp, regs[2], label, "x35")
	s.gauge(descBufferCharging, float64(regs[3]&1), label)
	s.state(descBufferState, descBufferStateInfo, bufferStates, regs[4], label)
	s.gauge(descBufferRelease, float64(regs[5]), label)
}

// signed interprets a register as two's complement (the controller's "int" type).
func signed(v uint16) int16 {
	return int16(v) //nolint:gosec // registers are 16 bit two's complement values
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// ValidateConfig checks the configured circuit, tank and buffer numbers.
func ValidateConfig(cfg Config) error {
	check := func(name string, ns []int, maxN int) error {
		for _, n := range ns {
			if n < 1 || n > maxN {
				return fmt.Errorf("%s %d out of range 1-%d", name, n, maxN)
			}
		}
		return nil
	}
	if err := check("heating circuit", cfg.HeatingCircuits, 8); err != nil {
		return err
	}
	if err := check("hot water tank", cfg.HotWaterTanks, 4); err != nil {
		return err
	}
	return check("buffer", cfg.Buffers, 4)
}
