package et

// sensorDefinition defines how to read a specific sensor from the inverter.
type sensorDefinition struct {
	Register uint16
	Scale    float64
}

// registry maps sensor names to their Modbus register and scaling factor.
var registry = map[string]sensorDefinition{
	"pv_power":           {Register: 0x0100, Scale: 0.1},
	"pv_voltage":         {Register: 0x0101, Scale: 0.1},
	"l1_voltage":         {Register: 0x0200, Scale: 0.1},
	"l2_voltage":         {Register: 0x0201, Scale: 0.1},
	"l3_voltage":         {Register: 0x0202, Scale: 0.1},
	"l1_current":         {Register: 0x0203, Scale: 0.01},
	"l2_current":         {Register: 0x0204, Scale: 0.01},
	"l3_current":         {Register: 0x0205, Scale: 0.01},
	"l1_power":           {Register: 0x0206, Scale: 0.1},
	"l2_power":           {Register: 0x0207, Scale: 0.1},
	"l3_power":           {Register: 0x0208, Scale: 0.1},
	"house_consumption":  {Register: 0x0300, Scale: 0.1},
}
