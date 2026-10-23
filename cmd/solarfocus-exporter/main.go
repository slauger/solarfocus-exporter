// Command solarfocus-exporter exposes a SOLARFOCUS ecomanager-touch heating controller
// (Modbus TCP) as Prometheus metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/slauger/solarfocus-exporter/internal/modbus"
	"github.com/slauger/solarfocus-exporter/internal/solarfocus"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("solarfocus-exporter", flag.ContinueOnError)
	var (
		listen   = fs.String("web.listen-address", envOr("SOLARFOCUS_LISTEN_ADDRESS", ":9851"), "address to serve metrics on")
		address  = fs.String("modbus.address", envOr("SOLARFOCUS_MODBUS_ADDRESS", ""), "host:port of the controller (Modbus TCP)")
		unitID   = fs.Uint("modbus.unit-id", 1, "Modbus unit id")
		timeout  = fs.Duration("modbus.timeout", 10*time.Second, "timeout for reading all registers of one scrape")
		circuits = fs.String("heating-circuits", envOr("SOLARFOCUS_HEATING_CIRCUITS", "1"), "comma separated heating circuit numbers (1-8)")
		tanks    = fs.String("hot-water-tanks", envOr("SOLARFOCUS_HOT_WATER_TANKS", "1"), "comma separated hot water tank numbers (1-4)")
		buffers  = fs.String("buffers", envOr("SOLARFOCUS_BUFFERS", ""), "comma separated buffer tank numbers (1-4)")
		showVer  = fs.Bool("version", false, "print version and exit")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVer {
		fmt.Println(version)
		return nil
	}
	if *address == "" {
		return errors.New("-modbus.address (or SOLARFOCUS_MODBUS_ADDRESS) is required")
	}
	if *unitID > 247 {
		return fmt.Errorf("invalid unit id %d", *unitID)
	}

	cfg := solarfocus.Config{Timeout: *timeout}
	var err error
	if cfg.HeatingCircuits, err = parseList(*circuits); err != nil {
		return fmt.Errorf("heating circuits: %w", err)
	}
	if cfg.HotWaterTanks, err = parseList(*tanks); err != nil {
		return fmt.Errorf("hot water tanks: %w", err)
	}
	if cfg.Buffers, err = parseList(*buffers); err != nil {
		return fmt.Errorf("buffers: %w", err)
	}
	if err := solarfocus.ValidateConfig(cfg); err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	client := modbus.NewClient(*address, byte(*unitID), *timeout)
	defer func() { _ = client.Close() }()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           newHandler(client, cfg, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting", "version", version, "listen", *listen, "modbus", *address,
			"heating_circuits", cfg.HeatingCircuits, "hot_water_tanks", cfg.HotWaterTanks, "buffers", cfg.Buffers)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// newHandler serves /metrics (registers read on every scrape), /healthz and /.
func newHandler(r solarfocus.RegisterReader, cfg solarfocus.Config, logger *slog.Logger) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		solarfocus.NewCollector(r, cfg, logger),
		collectors.NewBuildInfoCollector(),
		collectors.NewGoCollector(),
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("solarfocus-exporter - metrics at /metrics\n"))
	})
	return mux
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func parseList(s string) ([]int, error) {
	var out []int
	for f := range strings.SplitSeq(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", f)
		}
		out = append(out, n)
	}
	return out, nil
}
