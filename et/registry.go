package et

// sensorDefinition defines how to read a specific sensor from the bulk telemetry dump.
type sensorDefinition struct {
	Offset int
	Scale  float64
}

// registry maps sensor names to their offset in the bulk telemetry array and scaling factor.
var registry = map[string]sensorDefinition{
	"pv_power":           {Offset: 256, Scale: 0.1},
	"pv_voltage":         {Offset: 257, Scale: 0.1},
	"l1_voltage":         {Offset: 512, Scale: 0.1},
	"l2_voltage":         {Offset: 513, Scale: 0.1},
	"l3_voltage":         {Offset: 514, Scale: 0.1},
	"l1_current":         {Offset: 515, Scale: 0.01},
	"l2_current":         {Offset: 516, Scale: 0.01},
	"l3_current":         {Offset: 517, Scale: 0.01},
	"l1_power":           {Offset: 518, Scale: 0.1},
	"l2_power":           {Offset: 519, Scale: 0.1},
	"l3_power":           {Offset: 520, Scale: 0.1},
	"house_consumption":  {Offset: 768, Scale: 0.1},
}

// GetSensorNames returns a list of all available sensor names in the registry.
func GetSensorNames() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
