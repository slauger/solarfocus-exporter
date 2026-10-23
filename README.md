# solarfocus-exporter

Prometheus exporter for SOLARFOCUS heating systems with the **ecomanager-touch** controller (pellet boilers such as pellet<sup>elegance</sup>/octo<sup>plus</sup>, therminator, vamp<sup>air</sup>), read via **Modbus TCP**.

Minimal by design: a single static Go binary in a `scratch` image (~10 MB), one dependency (`prometheus/client_golang`), its own tiny Modbus TCP client and read-only access to the controller.

## Features

- Boiler (Kessel): temperature, status line incl. alarm state, active message number, door contact, ash box fill level, cleaning indicator, outdoor temperature
- Heating circuits 1-8: flow/room temperature, humidity, pump, mixer, limit thermostat, status, operating mode
- Domestic hot water tanks 1-4 ("Boiler"): temperature, target temperature, status, release mode, single charge
- Buffer tanks 1-4: top/bottom/X35 temperatures, charging, status
- Status codes as numbers **and** as German text (`*_info{state="..."}`) from the SOLARFOCUS register documentation
- Missing or defective sensors (e.g. 130 °C, -999, -1 %) are not exported instead of producing bogus values
- Registers are read on every scrape (no background polling)

## Prerequisites

Modbus TCP must be enabled on the controller (Freigabemenü → "Modbus TCP", administrator code required). Port 502, unit id 1.

## Usage

```bash
solarfocus-exporter -modbus.address 10.0.0.230:502
curl localhost:9851/metrics
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `-modbus.address` | `SOLARFOCUS_MODBUS_ADDRESS` | - | host:port of the controller (required) |
| `-modbus.unit-id` | | `1` | Modbus unit id |
| `-modbus.timeout` | | `10s` | timeout for one scrape |
| `-heating-circuits` | `SOLARFOCUS_HEATING_CIRCUITS` | `1` | heating circuits to read, e.g. `1,2` |
| `-hot-water-tanks` | `SOLARFOCUS_HOT_WATER_TANKS` | `1` | hot water tanks to read |
| `-buffers` | `SOLARFOCUS_BUFFERS` | (none) | buffer tanks to read |
| `-web.listen-address` | `SOLARFOCUS_LISTEN_ADDRESS` | `:9851` | listen address |

Endpoints: `/metrics`, `/healthz`.

A scrape takes about one second (the controller is slow); a scrape interval of 30-60s is recommended.

### Container

```bash
make image
docker run --rm -p 9851:9851 -e SOLARFOCUS_MODBUS_ADDRESS=10.0.0.230:502 ghcr.io/slauger/solarfocus-exporter:<version>
```

## Metrics

All metrics are gauges with the prefix `solarfocus_`.

| Metric | Labels | Description |
|--------|--------|-------------|
| `up` | | 1 if all configured registers were read |
| `scrape_duration_seconds` | | duration of the Modbus reads |
| `boiler_temperature_celsius` | | boiler temperature |
| `boiler_state`, `boiler_state_info` | `code`, `state` | status line (e.g. 2 = Pelletsbetrieb, 40 = Alarm aktiv!) |
| `boiler_message_number` | | active message on the controller (0 = none) |
| `boiler_door_open` | | 1 = door open |
| `boiler_ash_box_percent` | | ash box fill level, empty at 100 |
| `boiler_cleaning_percent` | | cleaning indicator, cleaning due at 100 |
| `boiler_fuel_type` | | therminator operating type |
| `outdoor_temperature_celsius` | | outdoor temperature |
| `heating_circuit_flow_temperature_celsius` | `circuit` | flow temperature |
| `heating_circuit_room_temperature_celsius` | `circuit` | room temperature (room sensor only) |
| `heating_circuit_humidity_percent` | `circuit` | humidity (sensor only) |
| `heating_circuit_pump_on` | `circuit` | 1 = pump running |
| `heating_circuit_mixer_percent` | `circuit` | mixer position |
| `heating_circuit_limit_thermostat_closed` | `circuit` | 1 = limit thermostat closed (ok) |
| `heating_circuit_state`, `heating_circuit_state_info` | `circuit`, `code`, `state` | circuit status |
| `heating_circuit_mode`, `heating_circuit_mode_info` | `circuit`, `code`, `mode` | operating mode (Dauer/Absenk/Automatik/Aus) |
| `heating_circuit_cooling` | `circuit` | 1 = cooling |
| `hot_water_temperature_celsius` | `tank` | hot water temperature |
| `hot_water_target_temperature_celsius` | `tank` | hot water target temperature |
| `hot_water_state`, `hot_water_state_info` | `tank`, `code`, `state` | hot water status |
| `hot_water_release_mode`, `hot_water_release_mode_info` | `tank`, `code`, `mode` | release mode |
| `hot_water_single_charge_active` | `tank` | 1 = single charge active |
| `buffer_temperature_celsius` | `buffer`, `sensor` | `top`, `bottom`, `x35` |
| `buffer_charging` | `buffer` | 1 = being charged |
| `buffer_state`, `buffer_state_info` | `buffer`, `code`, `state` | buffer status |
| `buffer_release_mode` | `buffer` | 0 off, 1 on, 2 timer |

Solar and heat pump registers are documented by SOLARFOCUS but not (yet) exported.

## Example alerts

```yaml
- alert: HeatingBoilerAlarm
  expr: solarfocus_boiler_state == 40 or solarfocus_boiler_message_number > 0
  for: 5m
  annotations:
    summary: "Heizung: {{ with query \"solarfocus_boiler_state_info\" }}{{ . | first | label \"state\" }}{{ end }}"
- alert: HeatingAshBoxFull
  expr: solarfocus_boiler_ash_box_percent >= 90
- alert: HeatingDoorOpen
  expr: solarfocus_boiler_door_open == 1
  for: 15m
- alert: HeatingHotWaterCold
  expr: solarfocus_hot_water_temperature_celsius < 40
  for: 30m
- alert: HeatingUnreachable
  expr: solarfocus_up == 0 or absent(solarfocus_up)
  for: 10m
```

## Development

```bash
make build test lint cover   # cover enforces 80 %
make ci                      # tidy, vet, lint, vulncheck, cover
```

Register reference: `Regelung ecomanager-touch: Modbus TCP - Registerdaten` (DR-0180-DE v1-201907). Holding registers 32600/32605 (flow/room target) are setpoints written via Modbus and read back as 0 when unused; they are therefore not exported.

## License

Apache-2.0
