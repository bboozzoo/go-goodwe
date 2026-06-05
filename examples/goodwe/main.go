package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bboozzoo/go-goodwe/et"
)

func main() {
	// Define flags
	ip := flag.String("ip", "", "IP address of the GoodWe inverter")
	interval := flag.Duration("interval", 5*time.Second, "Polling interval (e.g., 5s, 1m)")
	flag.Parse()

	// Validate input
	if *ip == "" {
		fmt.Println("Error: -ip is required")
		flag.Usage()
		os.Exit(1)
	}

	// Setup context with cancellation on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals to shut down gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		cancel()
	}()

	fmt.Printf("Initializing connection to ET Inverter at %s...\n", *ip)
	inverter := et.New(*ip)

	// 1. Connect
	if err := inverter.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Println("Connected successfully!")

	// Ensure cleanup happens
	defer func() {
		fmt.Println("Closing connection...")
		if err := inverter.Close(); err != nil {
			log.Printf("Error during close: %v", err)
		} else {
			fmt.Println("Connection closed cleanly.")
		}
	}()

	// 2. Get Device Info
	fmt.Println("\n--- Device Information ---")
	info, err := inverter.GetInfo(ctx)
	if err != nil {
		log.Printf("Could not retrieve device info: %v", err)
	} else {
		fmt.Printf("  Model:     %s\n", info.Model)
		fmt.Printf("  Serial:    %s\n", info.SerialNumber)
		fmt.Printf("  Firmware:  %s\n", info.Firmware)
	}

	// 3. Continuous Polling
	fmt.Printf("\n--- Starting sensor polling (every %v) ---\n", *interval)
	fmt.Println("Press Ctrl+C to stop.")
	
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Polling terminated.")
			return
		case <-time.After(*interval):
			sensors, err := inverter.GetSensors(ctx)
			if err != nil {
				log.Printf("Error reading sensors: %v", err)
				continue
			}

			fmt.Printf("[%s] ", time.Now().Format("15:04:05"))
			if len(sensors) == 0 {
				fmt.Print("No sensor data available.")
			} else {
				for name, val := range sensors {
					fmt.Printf("%s: %.2f | ", name, val)
				}
			}
			fmt.Println()
		}
	}
}
