// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Maciej Borzecki <maciej.borzecki@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the names of its contributors
//    may be used to endorse or promote products derived from this software
//    without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

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
	"runtime/debug"
	"syscall"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/discovery"
	"github.com/bboozzoo/go-goodwe/pkg/api"
	"github.com/bboozzoo/go-goodwe/pkg/daemon"
)

var version = "dev"

func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := s.Value
				if len(rev) > 7 {
					rev = rev[:7]
				}
				return fmt.Sprintf("%s (%s)", version, rev)
			}
		}
	}
	return version
}

const (
	minPollInterval     = 5 * time.Second
	shutdownTimeout     = 15 * time.Second
	httpShutdownTimeout = 10 * time.Second
)

func main() {
	daemonAddr := flag.String("daemon", "", "Address and port for the HTTP API server (e.g. :8080)")
	dashboard := flag.Bool("dashboard", false, "Enable the embedded JS dashboard at /dashboard")
	dsn := flag.String("dbstore", "", "Database connection string (e.g. sqlite://~/.goodwe/goodwe.db)")
	pollInterval := flag.Duration("poll", 0, "Sensor poll interval (e.g. 30s, 1m; minimum 5s)")
	inverterIP := flag.String("inverterip", "", "IP address of the GoodWe inverter")
	purgeDate := flag.String("purge", "", "One-shot: purge all data older than this date and exit")
	debug := flag.Bool("debug", false, "Enable debug logging")
	showVersion := flag.Bool("version", false, "Display version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(getVersion())
		os.Exit(0)
	}

	// Set up logging.
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if *daemonAddr == "" {
		fmt.Println("Error: -daemon <address>:<port> is required")
		flag.Usage()
		os.Exit(1)
	}

	// Enforce minimum poll interval.
	if *pollInterval > 0 && *pollInterval < minPollInterval {
		fmt.Printf("Error: -poll interval must be at least %s (got %s)\n", minPollInterval, *pollInterval)
		os.Exit(1)
	}

	if *dsn == "" {
		*dsn = "sqlite://~/.goodwe/goodwe.db"
	}

	// In the skeleton phase we don't use dbstore, dashboard, or purge yet.
	_ = dsn
	_ = dashboard
	_ = purgeDate

	slog.Info("Starting GoodWe daemon",
		"version", getVersion(),
		"listen", *daemonAddr,
		"dashboard", *dashboard,
		"poll", *pollInterval,
		"debug", *debug,
	)

	// Discover and connect to the inverter.
	var inverter goodwe.Inverter
	if *inverterIP != "" {
		slog.Info("Discovering inverter", "ip", *inverterIP)
		var err error
		inverter, err = discovery.Discover(context.Background(), *inverterIP)
		if err != nil {
			slog.Error("Failed to discover inverter", "error", err)
			os.Exit(1)
		}
		slog.Info("Discovered inverter")
	} else {
		slog.Warn("No -inverterip specified; sensor endpoints will return 503")
	}

	// Create the API handler with the inverter (may be nil).
	handler := api.New(inverter, *debug)

	// Create the daemon with the inverter (may be nil).
	dmn := daemon.New(inverter)

	// Start HTTP server.
	httpServer := &http.Server{
		Addr:    *daemonAddr,
		Handler: handler,
	}

	// Create a cancellable context for the shutdown sequence.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trap SIGINT/SIGTERM for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in a goroutine.
	go func() {
		slog.Info("HTTP server listening", "address", *daemonAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
			cancel() // trigger shutdown
		}
	}()

	// Start daemon poll loop in a goroutine.
	go func() {
		if err := dmn.Run(ctx); err != nil {
			slog.Error("Daemon error", "error", err)
			cancel()
		}
	}()

	// Wait for a signal.
	sig := <-sigChan
	slog.Info("Received signal, shutting down...", "signal", sig.String())
	cancel()

	// Execute orderly shutdown sequence.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// 1. Shut down HTTP server (drain in-flight requests).
	slog.Info("Stopping HTTP server...")
	httpShutdownCtx, httpShutdownCancel := context.WithTimeout(shutdownCtx, httpShutdownTimeout)
	defer httpShutdownCancel()
	if err := httpServer.Shutdown(httpShutdownCtx); err != nil {
		slog.Error("HTTP server shutdown timed out or failed", "error", err)
	} else {
		slog.Info("HTTP server stopped")
	}

	// 2. Close daemon resources.
	slog.Info("Closing daemon...")
	if err := dmn.Close(); err != nil {
		slog.Error("Daemon close error", "error", err)
	} else {
		slog.Info("Daemon closed")
	}

	slog.Info("GoodWe daemon shut down cleanly")
	os.Exit(0)
}
