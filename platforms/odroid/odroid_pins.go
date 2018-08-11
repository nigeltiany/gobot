package odroid

var DigitalPinMap = map[string]int{
	"4": 173,
	"5": 174,
	"6": 171,
	"7": 192,
	"8": 172,
	"9": 191,
	"10": 189,
	"11": 190,
	"13": 21,
	"14": 210,
	"15": 18,
	"16": 209,
	"17": 22,
	"18": 19,
	"19": 30,
	"20": 28,
	"21": 29,
	"22": 31,
	"24": 25,
	"25": 23,
	"26": 24,
	"27": 33,
	"[4]": 188,
	"[5]": 34,
	"[6]": 187,
}

var AnalogPinMap = map[string]string{
	"3": "in_voltage0_raw",
	"23": "in_voltage3_raw",
	"AIN0": "in_voltage0_raw",
	"AIN3": "in_voltage3_raw",
}