package et

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Expected sensor values from the Python test suite (GW10K-ET_running_data.hex).
// Python reference: https://github.com/bboozzoo/py-goodwe (MIT License)
//
// house_consumption formula per Python:
//   ppv1 + ppv2 + ppv3 + ppv4 + pbattery1 - active_power
//   = 1695 + 1761 + 0 + 0 + (-2512) - (-3)
//   = 947

func loadSampleHex(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to read testdata/%s: %v", name, err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}
	return b
}

func TestParseModbusBulkResponse_GW10K(t *testing.T) {
	raw := loadSampleHex(t, "GW10K-ET_running_data.hex")
	data, err := parseModbusBulkResponse(raw)
	if err != nil {
		t.Fatalf("parseModbusBulkResponse: %v", err)
	}
	if len(data) != 125 {
		t.Fatalf("expected 125 registers, got %d", len(data))
	}
	// Spot-check a few known register values from Python test
	tests := []struct {
		index int
		want  uint16
		name  string
	}{
		{0, 0x1508, "timestamp[0]"}, // register 35100
		{3, 0x0CFE, "vpv1 (35103)"}, // vpv1 raw = 0x0CFE=3326 → 332.6V
		{4, 0x0033, "ipv1 (35104)"}, // ipv1 raw = 0x33=51 → 5.1A
		{7, 0x0CFE, "vpv2 (35107)"},
		{8, 0x0035, "ipv2 (35108)"},
		{10, 0x06E1, "ppv2 high (35110)"},
		{21, 0x0959, "vgrid (35121)"}, // vgrid = 0x0959 = 2393 → 239.3V
		{25, 0x0150, "pgrid (35125)"}, // pgrid = 0x0150 = 336W
		{36, 0x0001, "grid_mode (35136)"},
	}
	for _, tt := range tests {
		if tt.index >= len(data) {
			t.Errorf("index %d out of range", tt.index)
			continue
		}
		if data[tt.index] != tt.want {
			t.Errorf("%s (index %d): got 0x%04X, want 0x%04X", tt.name, tt.index, data[tt.index], tt.want)
		}
	}
}

func TestHouseConsumption_GW10K(t *testing.T) {
	raw := loadSampleHex(t, "GW10K-ET_running_data.hex")
	data, err := parseModbusBulkResponse(raw)
	if err != nil {
		t.Fatalf("parseModbusBulkResponse: %v", err)
	}

	sensor := registry["house_consumption"]
	got := sensor.Calculator(data)

	// Python reference: house_consumption = 947 W
	// 1695 + 1761 + 0 + 0 + (-2512) - (-3) = 947
	want := 947.0
	if got != want {
		t.Errorf("house_consumption: got %.0f, want %.0f", got, want)
	}
}

func TestHouseConsumptionComponents_GW10K(t *testing.T) {
	raw := loadSampleHex(t, "GW10K-ET_running_data.hex")
	data, err := parseModbusBulkResponse(raw)
	if err != nil {
		t.Fatalf("parseModbusBulkResponse: %v", err)
	}

	// Individual components of house_consumption, per Python reference
	// ppv1 = read_bytes4(data, 35105, 0)
	ppv1 := readUint32(data, 5)
	if ppv1 == undef32 {
		ppv1 = 0
	}
	if ppv1 != 1695 {
		t.Errorf("ppv1: got %d, want 1695", ppv1)
	}

	// ppv2 = read_bytes4(data, 35109, 0)
	ppv2 := readUint32(data, 9)
	if ppv2 == undef32 {
		ppv2 = 0
	}
	if ppv2 != 1761 {
		t.Errorf("ppv2: got %d, want 1761", ppv2)
	}

	// ppv3 = read_bytes4(data, 35113, 0) — undefined (0xFFFFFFFF) → 0
	ppv3 := readUint32(data, 13)
	if ppv3 == undef32 {
		ppv3 = 0
	}
	if ppv3 != 0 {
		t.Errorf("ppv3: got %d, want 0 (undefined)", ppv3)
	}

	// ppv4 = read_bytes4(data, 35117, 0) — undefined (0xFFFFFFFF) → 0
	ppv4 := readUint32(data, 17)
	if ppv4 == undef32 {
		ppv4 = 0
	}
	if ppv4 != 0 {
		t.Errorf("ppv4: got %d, want 0 (undefined)", ppv4)
	}

	// pbattery1 = read_bytes4_signed(data, 35182)
	pbattery1 := int32(readUint32(data, 82))
	if pbattery1 != -2512 {
		t.Errorf("pbattery1: got %d, want -2512", pbattery1)
	}

	// active_power = read_bytes2_signed(data, 35140)
	activePower := int16(data[40])
	if activePower != -3 {
		t.Errorf("active_power: got %d, want -3", activePower)
	}
}

// TODO: Implement remaining sensors from Python reference.
// Expected values from GW10K-ET_running_data.hex (Python test):
//
//   vpv1=332.6 ipv1=5.1  ppv1=1695  vpv2=332.6 ipv2=5.3  ppv2=1761  ppv=3456
//   vgrid=239.3 igrid=1.5 fgrid=49.99 pgrid=336
//   vgrid2=241.5 igrid2=1.3 fgrid2=49.99 pgrid2=287
//   vgrid3=241.1 igrid3=1.1 fgrid3=49.99 pgrid3=206
//   total_inverter_power=831 active_power=-3
//   reactive_power=0 apparent_power=0
//   backup_v1=239.0 backup_i1=0.6 backup_f1=49.98
//   backup_p1=107 backup_v2=241.3 backup_i2=0.9 backup_f2=50.0
//   backup_p2=189 backup_v3=241.2 backup_i3=0.2 backup_f3=49.99
//   backup_p3=0 load_p1=224 load_p2=80 load_p3=233
//   load_ptotal=522 backup_ptotal=312 ups_load=4
//   temperature_air=51.0 temperature_module=0 temperature=58.7
//   bus_voltage=803.6 nbus_voltage=401.8
//   vbattery1=254.2 ibattery1=-9.8 pbattery1=-2512
//   e_total=6085.3 e_day=12.5 e_total_exp=4718.6 h_total=9246
//   e_day_exp=9.8 e_total_imp=58.0 e_day_imp=0
//   e_load_total=8820.2 e_load_day=11.6
//   e_bat_charge_total=2758.1 e_bat_charge_day=5.3
//   e_bat_discharge_total=2442.1 e_bat_discharge_day=2.9
//
// Register offsets (base 35100):
//   pv1 voltage: 3 (uint16, scale 0.1)
//   pv1 current: 4 (uint16, scale 0.1)
//   pv1 power:   5 (uint32)
//   pv2 voltage: 7 (uint16, scale 0.1)
//   pv2 current: 8 (uint16, scale 0.1)
//   pv2 power:   9 (uint32)
//   pv3 power:   13 (uint32)
//   pv4 power:   17 (uint32)
//   grid L1 voltage: 21 (uint16, scale 0.1)
//   grid L1 current: 22 (uint16, scale 0.1)
//   grid L1 freq:    23 (int16, scale 0.01)
//   grid L1 power:   25 (int16)
//   grid L2 voltage: 26 (uint16, scale 0.1)
//   ...
//   battery power:   82 (int32)
//   active power:    40 (int16)
//   house_consumption: calculated (ppv1+ppv2+ppv3+ppv4+pbattery1-active_power)
