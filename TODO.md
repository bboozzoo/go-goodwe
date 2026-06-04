## TODOs

List of TODO items:

- python/goodwe as reference
- example/reference a command line client under examples/cmd/goodwe
  - see provided main.go as usage template
- examples/output.log contains sensors read from GW12K-ET-20 inverter

Rules:
- code must be 'gofmt' clean
- use golangci-lint to verify that the code is clean


## Goals

### Discovery

```sh
$ goodwe discover
devices:
  - model: GWL-20L
    serial: 1231231 
    address: 192.168.100.151
```

### Info

```sh
goodwe device-info 192.168.100.151 [--family ET] [--sensors-all] 
<connects to the device>
<figures out the protocol family>
<figures out the correct port and whether DTLS is needed>
model: GWL-20L
serial: 1231231
address: 192.168.100.151
operational-status: on-grid
sensors:  # list of sensors
   grid_mode: <val>
   timestamp: 2026-06-04 21:42:58  # should be RFC3339
   # only important sensors like operation mode, export, import, production
   # unless --all was passed
```

read one sensor:

```sh
goodwe read-sensor 192.168.100.151 <sensor-id> 
grid_mode: <val>
timestamp: 2026-06-04 21:42:58  # should be RFC3339
```

polling

```sh
goodwe monitor 192.168.100.151 --time 5s <sensor-id> [<sensor-id>] 
timestamp: 2026-06-04 21:42:58  # should be RFC3339
grid_mode: <val>

timestamp: 2026-06-04 21:43:03
grid_mode: <val>

# another readout
```
