package et

const (
	undef16 uint16 = 0xFFFF
	undef32 uint32 = 0xFFFFFFFF
)

// sensorDefinition defines how to read a specific sensor from the inverter.
type sensorDefinition struct {
	Calculator func(data []uint16) float64
}

// registry maps sensor names to their definitions (all offsets are relative to base register 35100).
var registry = map[string]sensorDefinition{
	// PV strings
	"vpv1":  {Calculator: uint16Reader(3, 0.1)},
	"ipv1":  {Calculator: uint16Reader(4, 0.1)},
	"ppv1":  {Calculator: uint32Reader(5)},
	"vpv2":  {Calculator: uint16Reader(7, 0.1)},
	"ipv2":  {Calculator: uint16Reader(8, 0.1)},
	"ppv2":  {Calculator: uint32Reader(9)},
	"vpv3":  {Calculator: uint16Reader(11, 0.1)},
	"ipv3":  {Calculator: uint16Reader(12, 0.1)},
	"ppv3":  {Calculator: uint32Reader(13)},
	"vpv4":  {Calculator: uint16Reader(15, 0.1)},
	"ipv4":  {Calculator: uint16Reader(16, 0.1)},
	"ppv4":  {Calculator: uint32Reader(17)},

	// Grid (L1)
	"vgrid":                {Calculator: uint16Reader(21, 0.1)},
	"igrid":                {Calculator: uint16Reader(22, 0.1)},
	"fgrid":                {Calculator: int16Reader(23, 0.01)},
	"pgrid":                {Calculator: int16Reader(25, 1)},
	"vgrid2":               {Calculator: uint16Reader(26, 0.1)},
	"igrid2":               {Calculator: uint16Reader(27, 0.1)},
	"fgrid2":               {Calculator: int16Reader(28, 0.01)},
	"pgrid2":               {Calculator: int16Reader(30, 1)},
	"vgrid3":               {Calculator: uint16Reader(31, 0.1)},
	"igrid3":               {Calculator: uint16Reader(32, 0.1)},
	"fgrid3":               {Calculator: int16Reader(33, 0.01)},
	"pgrid3":               {Calculator: int16Reader(35, 1)},
	"grid_mode":            {Calculator: uint16Reader(36, 1)},
	"total_inverter_power": {Calculator: int16Reader(38, 1)},
	"active_power":         {Calculator: int16Reader(40, 1)},
	"reactive_power":       {Calculator: int16Reader(42, 1)},
	"apparent_power":       {Calculator: int16Reader(44, 1)},

	// Backup/UPS
	"backup_v1":    {Calculator: uint16Reader(45, 0.1)},
	"backup_i1":    {Calculator: uint16Reader(46, 0.1)},
	"backup_f1":    {Calculator: int16Reader(47, 0.01)},
	"load_mode1":   {Calculator: uint16Reader(48, 1)},
	"backup_p1":    {Calculator: int16Reader(50, 1)},
	"backup_v2":    {Calculator: uint16Reader(51, 0.1)},
	"backup_i2":    {Calculator: uint16Reader(52, 0.1)},
	"backup_f2":    {Calculator: int16Reader(53, 0.01)},
	"load_mode2":   {Calculator: uint16Reader(54, 1)},
	"backup_p2":    {Calculator: int16Reader(56, 1)},
	"backup_v3":    {Calculator: uint16Reader(57, 0.1)},
	"backup_i3":    {Calculator: uint16Reader(58, 0.1)},
	"backup_f3":    {Calculator: int16Reader(59, 0.01)},
	"load_mode3":   {Calculator: uint16Reader(60, 1)},
	"backup_p3":    {Calculator: int16Reader(62, 1)},
	"load_p1":      {Calculator: int16Reader(64, 1)},
	"load_p2":      {Calculator: int16Reader(66, 1)},
	"load_p3":      {Calculator: int16Reader(68, 1)},
	"backup_ptotal": {Calculator: int16Reader(70, 1)},
	"load_ptotal":  {Calculator: int16Reader(72, 1)},
	"ups_load":     {Calculator: uint16Reader(73, 1)},

	// Temperature
	"temperature_air":    {Calculator: int16Reader(74, 0.1)},
	"temperature_module": {Calculator: int16Reader(75, 0.1)},
	"temperature":        {Calculator: int16Reader(76, 0.1)},

	// Internal
	"function_bit":  {Calculator: uint16Reader(77, 1)},
	"bus_voltage":   {Calculator: uint16Reader(78, 0.1)},
	"nbus_voltage":  {Calculator: uint16Reader(79, 0.1)},

	// Battery
	"vbattery1":     {Calculator: uint16Reader(80, 0.1)},
	"ibattery1":     {Calculator: int16Reader(81, 0.1)},
	"pbattery1":     {Calculator: int32Reader(82)},
	"battery_mode":  {Calculator: uint16Reader(84, 1)},
	"warning_code":  {Calculator: uint16Reader(85, 1)},
	"safety_country": {Calculator: uint16Reader(86, 1)},
	"work_mode":     {Calculator: uint16Reader(87, 1)},
	"operation_mode": {Calculator: uint16Reader(88, 1)},
	"error_codes":   {Calculator: uint32Reader(89)},

	// Energy totals
	"e_total":              {Calculator: energy4Reader(91)},
	"e_day":                {Calculator: energy4Reader(93)},
	"e_total_exp":          {Calculator: energy4Reader(95)},
	"h_total":              {Calculator: uint32Reader(97)},
	"e_day_exp":            {Calculator: energy2Reader(99)},
	"e_total_imp":          {Calculator: energy4Reader(100)},
	"e_day_imp":            {Calculator: energy2Reader(102)},
	"e_load_total":         {Calculator: energy4Reader(103)},
	"e_load_day":           {Calculator: energy2Reader(105)},
	"e_bat_charge_total":   {Calculator: energy4Reader(106)},
	"e_bat_charge_day":     {Calculator: energy2Reader(108)},
	"e_bat_discharge_total": {Calculator: energy4Reader(109)},
	"e_bat_discharge_day":  {Calculator: energy2Reader(111)},
	"diagnose_result":      {Calculator: uint32Reader(120)},

	// Calculated
	"ppv": {
		Calculator: func(data []uint16) float64 {
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
			return float64(v1 + v2 + v3 + v4)
		},
	},
	"house_consumption": {
		Calculator: func(data []uint16) float64 {
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

// Reader helpers for common sensor types.

func uint16Reader(offset int, scale float64) func([]uint16) float64 {
	return func(data []uint16) float64 {
		if offset >= len(data) {
			return 0
		}
		raw := data[offset]
		if raw == undef16 {
			return 0
		}
		return float64(raw) * scale
	}
}

func int16Reader(offset int, scale float64) func([]uint16) float64 {
	return func(data []uint16) float64 {
		if offset >= len(data) {
			return 0
		}
		return float64(int16(data[offset])) * scale
	}
}

func uint32Reader(offset int) func([]uint16) float64 {
	return func(data []uint16) float64 {
		v := readUint32(data, offset)
		if v == undef32 {
			return 0
		}
		return float64(v)
	}
}

func int32Reader(offset int) func([]uint16) float64 {
	return func(data []uint16) float64 {
		return float64(int32(readUint32(data, offset)))
	}
}

func energy4Reader(offset int) func([]uint16) float64 {
	return func(data []uint16) float64 {
		v := readUint32(data, offset)
		if v == undef32 {
			return 0
		}
		return float64(v) / 10.0
	}
}

func energy2Reader(offset int) func([]uint16) float64 {
	return func(data []uint16) float64 {
		if offset >= len(data) {
			return 0
		}
		raw := data[offset]
		if raw == undef16 {
			return 0
		}
		return float64(raw) / 10.0
	}
}

// Helpers for calculation

func readUint32(data []uint16, idx int) uint32 {
	if idx+1 >= len(data) {
		return 0
	}
	return uint32(data[idx])<<16 | uint32(data[idx+1])
}