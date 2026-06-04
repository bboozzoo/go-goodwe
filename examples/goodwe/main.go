package main

import (
	"context"
	"time"

	"github.com/bboozzoo/go-goodwe"
)

func discover(ctx context.Context, netInterfaceName string) {
	timeout := 10 * time.Second
	found, err := goodwe.Discover(ctx, netInteraceName, timeout)
	// erro rhandling

	for _, f := range found {
		fmt.Printf("%v: %v", f.Name, f.IPAddress)
	}
}

func connect(ctx context.Context, address string) {
	c, err := goodwe.Connect(ctx, address, goodwe.ProtocolOptions{})

	defer c.Close()

	// usage
	// obtain device info
	di, err := c.GetDeviceInfo(ctx)

	// obtain sensors data
	sensorsData, err := c.GetSensors()

	sensorsData, err := c.GetSensors()

}
