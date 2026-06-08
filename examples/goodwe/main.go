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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/discovery"
	"github.com/bboozzoo/go-goodwe/et"
)

func formatSensorValue(v any) string {
	switch val := v.(type) {
	case float64:
		return fmt.Sprintf("%.2f", val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func formatSensorOutput(name string, sv goodwe.SensorValue) string {
	valStr := formatSensorValue(sv.Value)
	if sv.Unit != "" {
		return fmt.Sprintf("%s: %s %s", name, valStr, sv.Unit)
	}
	return fmt.Sprintf("%s: %s", name, valStr)
}

func main() {
	ip := flag.String("ip", "", "IP address of the GoodWe inverter")
	pollInterval := flag.Duration("poll", 0, "Polling interval (e.g., 5s, 1m). If 0, polling is disabled.")
	readSensor := flag.String("readsensor", "", "Comma-separated sensor names to read (e.g. battery_soc,house_consumption). With -poll, these are polled alongside the timestamp.")
	listSensors := flag.Bool("listsensors", false, "List all available sensors and exit.")
	showInfo := flag.Bool("info", false, "Display inverter information and exit.")
	verbose := flag.Bool("verbose", false, "Enable info logging.")
	debug := flag.Bool("debug", false, "Enable debug logging (implies -verbose).")
	flag.Parse()

	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	} else if *verbose {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if *ip == "" {
		fmt.Println("Error: -ip is required")
		flag.Usage()
		os.Exit(1)
	}

	if *listSensors {
		groups := et.GetSensorNamesByBlock()
		fmt.Println("Available sensors:")
		for _, block := range []string{"Main Telemetry", "Battery", "Meter", "MPPT"} {
			names := groups[block]
			if len(names) == 0 {
				continue
			}
			fmt.Printf("\n %s:\n", block)
			for _, s := range names {
				fmt.Printf("  - %s\n", s)
			}
		}
		os.Exit(0)
	}

	if *pollInterval > 0 && *pollInterval < 5*time.Second {
		fmt.Println("Error: -poll interval must be at least 5s")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info("Received interrupt signal", "signal", sig.String())
		cancel()
	}()

	slog.Info("Discovering inverter", "ip", *ip)
	inverter, err := discovery.Discover(ctx, *ip)
	if err != nil {
		slog.Error("Failed to discover inverter", "error", err)
		os.Exit(1)
	}

	if err := inverter.Connect(ctx); err != nil {
		slog.Error("Failed to connect", "error", err)
		os.Exit(1)
	}
	slog.Info("Connected successfully")

	defer func() {
		slog.Info("Closing connection...")
		if err := inverter.Close(); err != nil {
			slog.Error("Error during close", "error", err)
		} else {
			slog.Info("Connection closed cleanly")
		}
	}()

	slog.Info("--- Device Information ---")
	info, err := inverter.GetInfo(ctx)
	if err != nil {
		slog.Warn("Could not retrieve device info", "error", err)
	} else {
		slog.Info("Device Info", "model", info.Model, "serial", info.SerialNumber, "firmware", info.Firmware)
	}

	if *showInfo {
		fmt.Println("Inverter Information:")
		fmt.Printf("  Serial:     %s\n", info.SerialNumber)
		fmt.Printf("  Model:      %s\n", info.Model)
		fmt.Printf("  Firmware:   %s\n", info.Firmware)
		fmt.Printf("  DSP:        %s\n", info.DSPVersion)
		fmt.Printf("  ARM:        %s\n", info.ARMVersion)
		if info.RatedPower > 0 {
			fmt.Printf("  Rated:      %d W\n", info.RatedPower)
		}
		modeVal, err := inverter.ReadSensor(ctx, "work_mode_label")
		if err == nil {
			fmt.Printf("  Mode:       %s\n", modeVal.Value)
		}
		os.Exit(0)
	}

	var pollSensorNames []string
	if *readSensor != "" {
		pollSensorNames = strings.Split(*readSensor, ",")
		for i := range pollSensorNames {
			pollSensorNames[i] = strings.TrimSpace(pollSensorNames[i])
		}
	}

	// Single read (no polling)
	if len(pollSensorNames) > 0 && *pollInterval == 0 {
		for _, name := range pollSensorNames {
			val, err := inverter.ReadSensor(ctx, name)
			if err != nil {
				slog.Error("Failed to read sensor", "sensor", name, "error", err)
				os.Exit(1)
			}
			fmt.Println(formatSensorOutput(name, val))
		}
		os.Exit(0)
	}

	// Polling loop
	if *pollInterval > 0 {
		slog.Info("--- Starting sensor polling", "interval", pollInterval)
		fmt.Println("Press Ctrl+C to stop.")

		for {
			select {
			case <-ctx.Done():
				slog.Info("Polling terminated by user")
				return
			case <-time.After(*pollInterval):
				fmt.Printf("timestamp: %s\n", time.Now().Format(time.RFC3339))
				for _, name := range pollSensorNames {
					val, err := inverter.ReadSensor(ctx, name)
					if err != nil {
						slog.Error("Failed to read sensor", "sensor", name, "error", err)
						continue
					}
					fmt.Println(formatSensorOutput(name, val))
				}
				fmt.Println()
			}
		}
	} else {
		slog.Info("No polling requested (use -poll <time>). Exiting.")
	}
}
