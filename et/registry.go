package et

const (
	undef16 uint16 = 0xFFFF
	undef32 uint32 = 0xFFFFFFFF
)

// sensorDefinition defines how to read a specific sensor from the inverter.
type sensorDefinition struct {
	Offset     int                         // For direct register reads
	Scale      float64                     // For direct register reads
	Calculator func(data []uint16) float64 // For synthetic/calculated values
}

// registry maps sensor names to their definitions.
var registry = map[string]sensorDefinition{
	// Direct mappings (using offsets from base 35100)
	// Note: These offsets are illustrative until the full map is verified.
	"pv_power":   {Offset: 256, Scale: 0.1},
	"pv_voltage": {Offset: 257, Scale: 0.1},
	"l1_voltage": {Offset: 512, Scale: 0.1},
	"l2_voltage": {Offset: 513, Scale: 0.1},
	"l3_voltage": {Offset: 514, Scale: 0.1},
	"l1_current": {Offset: 515, Scale: 0.01},
	"l2_current": {Offset: 516, Scale: 0.01},
	"l3_current": {Offset: 517, Scale: 0.01},
	"l1_power":   {Offset: 518, Scale: 0.1},
	"l2_power":   {Offset: 519, Scale: 0.1},
	"l3_power":   {Offset: 520, Scale: 0.1},

	// Calculated mappings
	"house_consumption": {
		Calculator: func(data []uint16) float64 {
			// Python: read_bytes4(35105, 0) + read_bytes4(35109, 0) + read_bytes4(35113, 0) + read_bytes4(35117, 0) + read_bytes4_signed(35182) - read_bytes2_signed(35140)
			// Base 35100. The 3rd arg to read_bytes4 is undef=0, so 0xFFFFFFFF becomes 0.
			v1 := readUint32(data, 5)
			if v1 == undef32 {
				v1 = 0
			}
			v2 := readUint32(data, 9)
			if v2 == undef32 {
				v2 = 0
			}
			v3 := readUint32(data, 13)
			if v3 == undef32 {
				v3 = 0
			}
			v4 := readUint32(data, 17)
			if v4 == undef32 {
				v4 = 0
			}
			v5 := int32(readUint32(data, 82))
			v6 := int16(data[40])

			return float64(int64(v1) + int64(v2) + int64(v3) + int64(v4) + int64(v5) - int64(v6))
		},
	},
}

// GetSensorNames returns a list of all available sensor names in the registry.
func GetSensorNames() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Helpers for calculation

func readUint32(data []uint16, idx int) uint32 {
	if idx+1 >= len(data) {
		return 0
	}
	return uint32(data[idx])<<16 | uint32(data[idx+1])
}

func readInt32(data []uint16, idx int) int32 {
	return int32(readUint32(data, idx))
}

func readInt16(data []uint16, idx int) int16 {
	if idx >= len(data) {
		return 0
	}
	return int16(data[idx])
}
