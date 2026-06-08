Go package for interacting with GoodWe inverters.

## Usage

Build the CLI:

```sh
$ go build ./examples/goodwe/
```

Display inverter info:

```sh
$ ./goodwe -ip 192.168.100.151 -info
```

List all available sensors:

```sh
$ ./goodwe -ip 192.168.100.151 -listsensors
```

Read specific sensors (comma-separated):

```sh
$ ./goodwe -ip 192.168.100.151 -readsensor battery_soc,house_consumption,ppv
```

Poll sensors at a fixed interval:

```sh
$ ./goodwe -ip 192.168.100.151 -poll 10s -readsensor ppv,work_mode_label
```

Enable verbose or debug logging:

```sh
$ ./goodwe -ip 192.168.100.151 -info -verbose
$ ./goodwe -ip 192.168.100.151 -info -debug
```

### Troubleshooting

Identify open ports. Example from a rotuer which only supports RTU over UDP:

``` sh
$ sudo nmap -sS -sU -p T:502,U:8899,U:48899 192.168.100.151  # replace with your inverter's IP 
Starting Nmap 7.95 ( https://nmap.org ) at 2026-06-07 11:53 CEST
Nmap scan report for GW_WIFILAN_2 (192.168.100.151)
Host is up (0.093s latency).

PORT      STATE         SERVICE
502/tcp   closed        mbap
8899/udp  open|filtered ospf-lite
48899/udp open|filtered tc_ads_discovery
MAC Address: 28:56:2F:A8:EA:EC (Unknown)

Nmap done: 1 IP address (1 host up) scanned in 2.28 seconds
```

To query the inverter:

``` sh
$ echo -n "WIFIKIT-214028-READ" | nc -u -w 2 192.168.100.151 48899
dongle@sn,dtls_port:8899,<inverter's SN>
```

And another inverter which uses WIFI+LAN Kit:

``` sh
$ sudo nmap -sS -sU -p T:502,U:8899,U:48899 192.168.4.82
[sudo] password for maciek:
Starting Nmap 7.95 ( https://nmap.org ) at 2026-06-08 19:40 CEST
Nmap scan report for GW_WIFILAN_2 (192.168.4.82)
Host is up (0.074s latency).

PORT      STATE         SERVICE
502/tcp   open          mbap
8899/udp  closed        ospf-lite
48899/udp open|filtered tc_ads_discovery

Nmap done: 1 IP address (1 host up) scanned in 3.36 seconds

```

And the probe:
```sh
$ echo -n "WIFIKIT-214028-READ" | nc -u -w 2 192.168.4.82 48899
ccm@sn,ccm@sn,Solar-<sn>
```

### Links

Based on a super useful Python library: https://github.com/marcelblijleven/goodwe
