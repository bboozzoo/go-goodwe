package et

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expected sensor values from the Python test suite.
// Python reference: https://github.com/bboozzoo/py-goodwe (MIT License)

func loadSampleHex(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "failed to read testdata/%s", name)
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	require.NoError(t, err, "failed to decode hex")
	return b
}

func parseSampleData(t *testing.T, name string) []uint16 {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	require.Len(t, data, 125, "expected 125 registers")
	return data
}

func TestParseModbusBulkResponse_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	tests := []struct {
		index int
		want  uint16
		name  string
	}{
		{0, 0x1508, "timestamp[0]"},
		{3, 0x0CFE, "vpv1 (35103)"},
		{4, 0x0033, "ipv1 (35104)"},
		{7, 0x0CFE, "vpv2 (35107)"},
		{8, 0x0035, "ipv2 (35108)"},
		{10, 0x06E1, "ppv2 high (35110)"},
		{21, 0x0959, "vgrid (35121)"},
		{25, 0x0150, "pgrid (35125)"},
		{36, 0x0001, "grid_mode (35136)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, data[tt.index], "index %d", tt.index)
		})
	}
}

func TestParseModbusBulkResponse_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	tests := []struct {
		index int
		want  uint16
		name  string
	}{
		{0, 0x1A03, "timestamp[0]"},
		{3, 0x192B, "vpv1 (35103)"},
		{4, 0x0011, "ipv1 (35104)"},
		{6, 0x044A, "ppv1 high (35106)"},
		{7, 0x192B, "vpv2 (35107)"},
		{10, 0x03E5, "ppv2 high (35110)"},
		{11, 0x163D, "vpv3 (35111)"},
		{15, 0x163D, "vpv4 (35115)"},
		{25, 0x028B, "pgrid (35125)"},
		{36, 0x0001, "grid_mode (35136)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, data[tt.index], "index %d", tt.index)
		})
	}
}

func houseConsumptionValue(t *testing.T, data []uint16) float64 {
	t.Helper()
	sensor, ok := registry["house_consumption"]
	require.True(t, ok, "house_consumption not in registry")
	return sensor.Calculator(data)
}

func TestHouseConsumption_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	assert.Equal(t, 947.0, houseConsumptionValue(t, data))
}

func TestHouseConsumptionComponents_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")

	ppv1 := readUint32(data, 5)
	if ppv1 == undef32 {
		ppv1 = 0
	}
	assert.EqualValues(t, 1695, ppv1, "ppv1")

	ppv2 := readUint32(data, 9)
	if ppv2 == undef32 {
		ppv2 = 0
	}
	assert.EqualValues(t, 1761, ppv2, "ppv2")

	ppv3 := readUint32(data, 13)
	if ppv3 == undef32 {
		ppv3 = 0
	}
	assert.EqualValues(t, 0, ppv3, "ppv3 (undefined)")

	ppv4 := readUint32(data, 17)
	if ppv4 == undef32 {
		ppv4 = 0
	}
	assert.EqualValues(t, 0, ppv4, "ppv4 (undefined)")

	assert.Equal(t, int32(-2512), int32(readUint32(data, 82)), "pbattery1")
	assert.Equal(t, int16(-3), int16(data[40]), "active_power")
}

func TestHouseConsumption_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	assert.Equal(t, 386.0, houseConsumptionValue(t, data))
}

func TestHouseConsumptionComponents_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")

	ppv1 := readUint32(data, 5)
	if ppv1 == undef32 {
		ppv1 = 0
	}
	assert.EqualValues(t, 1098, ppv1, "ppv1")

	ppv2 := readUint32(data, 9)
	if ppv2 == undef32 {
		ppv2 = 0
	}
	assert.EqualValues(t, 997, ppv2, "ppv2")

	ppv3 := readUint32(data, 13)
	if ppv3 == undef32 {
		ppv3 = 0
	}
	assert.EqualValues(t, 0, ppv3, "ppv3")

	ppv4 := readUint32(data, 17)
	if ppv4 == undef32 {
		ppv4 = 0
	}
	assert.EqualValues(t, 0, ppv4, "ppv4")

	assert.Equal(t, int32(-153), int32(readUint32(data, 82)), "pbattery1")
	assert.Equal(t, int16(1556), int16(data[40]), "active_power")
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
