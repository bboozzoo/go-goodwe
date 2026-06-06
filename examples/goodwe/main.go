// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Maciej Borzecki <maciek.borzecki@gmail.com>
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
	"sort"
	"syscall"
	"time"

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

func formatSensorOutput(name string, val any) string {
	return fmt.Sprintf("%s: %s", name, formatSensorValue(val))
}

func main() {
	// Define flags
	ip := flag.String("ip", "", "IP address of the GoodWe inverter")
	pollInterval := flag.Duration("poll", 0, "Polling interval (e.g., 5s, 1m). If 0, polling is disabled.")
	readSensor := flag.String("readsensor", "", "Name of the specific sensor to read and exit.")
	listSensors := flag.Bool("listsensors", false, "List all available sensors and exit.")
	flag.Parse()

	// Setup slog
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Validate input
	if *ip == "" {
		fmt.Println("Error: -ip is required")
		flag.Usage()
		os.Exit(1)
	}

	// Handle list sensors request
	if *listSensors {
		sensors := et.GetSensorNames()
		sort.Strings(sensors)
		fmt.Println("Available sensors:")
		for _, s := range sensors {
			fmt.Printf(" - %s\n", s)
		}
		os.Exit(0)
	}

	// Setup context with cancellation on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals to shut down gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info("Received interrupt signal", "signal", sig.String())
		cancel()
	}()

	slog.Info("Initializing connection", "ip", *ip)
	inverter := et.New(*ip)

	// 1. Connect
	if err := inverter.Connect(ctx); err != nil {
		slog.Error("Failed to connect", "error", err)
		os.Exit(1)
	}
	slog.Info("Connected successfully")

	// Ensure cleanup happens
	defer func() {
		slog.Info("Closing connection...")
		if err := inverter.Close(); err != nil {
			slog.Error("Error during close", "error", err)
		} else {
			slog.Info("Connection closed cleanly")
		}
	}()

	// 2. Get Device Info
	slog.Info("--- Device Information ---")
	info, err := inverter.GetInfo(ctx)
	if err != nil {
		slog.Warn("Could not retrieve device info", "error", err)
	} else {
		slog.Info("Device Info", "model", info.Model, "serial", info.SerialNumber, "firmware", info.Firmware)
	}

	// 3. Handle specific sensor read request
	if *readSensor != "" {
		slog.Info("Single sensor read requested", "sensor", *readSensor)
		sensors, err := inverter.GetSensors(ctx)
		if err != nil {
			slog.Error("Failed to read sensors", "error", err)
			os.Exit(1)
		}
		if val, ok := sensors[*readSensor]; ok {
			fmt.Println(formatSensorOutput(*readSensor, val))
			os.Exit(0)
		} else {
			slog.Error("Sensor not found", "sensor", *readSensor)
			os.Exit(1)
		}
	}

	// 4. Polling loop (only if -poll is provided)
	if *pollInterval > 0 {
		slog.Info("--- Starting sensor polling", "interval", pollInterval)
		fmt.Println("Press Ctrl+C to stop.")

		for {
			select {
			case <-ctx.Done():
				slog.Info("Polling terminated by user")
				return
			case <-time.After(*pollInterval):
				sensors, err := inverter.GetSensors(ctx)
				if err != nil {
					slog.Error("Error reading sensors", "error", err)
					continue
				}

				fmt.Printf("[%s] ", time.Now().Format("15:04:05"))
				if len(sensors) == 0 {
					fmt.Print("No sensor data available.")
				} else {
					for name, val := range sensors {
						fmt.Print(formatSensorOutput(name, val) + " | ")
					}
				}
				fmt.Println()
			}
		}
	} else {
		slog.Info("No polling requested (use -poll <time>). Exiting.")
	}
}
