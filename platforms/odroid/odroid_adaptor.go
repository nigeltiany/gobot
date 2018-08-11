package odroid

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/drivers/i2c"
	"gobot.io/x/gobot/drivers/spi"
	"gobot.io/x/gobot/sysfs"
)

type pwmPinData struct {
	channel int
	path    string
}

const pwmDefaultPeriod = 500000

// Adaptor is the gobot.Adaptor representation for the Odroid XU4
type Adaptor struct {
	name               string
	digitalPins        []*sysfs.DigitalPin
	pwmPins            map[string]*sysfs.PWMPin
	i2cBuses           map[int]i2c.I2cDevice
	usrLed             string
	analogPath         string
	pinMap             map[string]int
	analogPinMap       map[string]string
	mutex              *sync.Mutex
	findPin            func(pinPath string) (string, error)
	spiDefaultBus      int
	spiDefaultChip     int
	spiBuses           [2]spi.Connection
	spiDefaultMode     int
	spiDefaultMaxSpeed int64
}

// NewAdaptor returns a new Odroid Adaptor
func NewAdaptor() *Adaptor {
	b := &Adaptor{
		name:         gobot.DefaultName("Odroid-XU4"),
		digitalPins:  make([]*sysfs.DigitalPin, 120),
		pwmPins:      make(map[string]*sysfs.PWMPin),
		i2cBuses:     make(map[int]i2c.I2cDevice),
		mutex:        &sync.Mutex{},
		pinMap:       DigitalPinMap,
		analogPinMap: AnalogPinMap,
		findPin: func(pinPath string) (string, error) {
			files, err := filepath.Glob(pinPath)
			return files[0], err
		},
	}

	b.setPaths()
	return b
}

func (o *Adaptor) setPaths() {
	o.analogPath = "/sys/devices/12d10000.adc/iio:device0/"
	o.spiDefaultBus = 0
	o.spiDefaultMode = 0
	o.spiDefaultMaxSpeed = 500000
}

// Name returns the Adaptor name
func (b *Adaptor) Name() string { return b.name }

// SetName sets the Adaptor name
func (b *Adaptor) SetName(n string) { b.name = n }

// Connect initializes the pwm and analog dts.
func (b *Adaptor) Connect() error {
	return nil
}

// Finalize releases all i2c devices and exported analog, digital, pwm pins.
func (b *Adaptor) Finalize() (err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, pin := range b.digitalPins {
		if pin != nil {
			if e := pin.Unexport(); e != nil {
				err = multierror.Append(err, e)
			}
		}
	}
	for _, pin := range b.pwmPins {
		if pin != nil {
			if e := pin.Unexport(); e != nil {
				err = multierror.Append(err, e)
			}
		}
	}
	for _, bus := range b.i2cBuses {
		if bus != nil {
			if e := bus.Close(); e != nil {
				err = multierror.Append(err, e)
			}
		}
	}
	for _, bus := range b.spiBuses {
		if bus != nil {
			if e := bus.Close(); e != nil {
				err = multierror.Append(err, e)
			}
		}
	}
	return
}

// DigitalRead returns a digital value from specified pin
func (o *Adaptor) DigitalRead(pin string) (val int, err error) {
	sysfsPin, err := o.DigitalPin(pin, sysfs.IN)
	if err != nil {
		return
	}
	return sysfsPin.Read()
}

// DigitalWrite writes a digital value to specified pin.
func (o *Adaptor) DigitalWrite(pin string, val byte) (err error) {
	sysfsPin, err := o.DigitalPin(pin, sysfs.OUT)
	if err != nil {
		return err
	}
	return sysfsPin.Write(int(val))
}

// DigitalPin retrieves digital pin value by name
func (o *Adaptor) DigitalPin(pin string, dir string) (sysfsPin sysfs.DigitalPinner, err error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	i, err := o.translatePin(pin)
	if err != nil {
		return
	}
	if o.digitalPins[i] == nil {
		o.digitalPins[i] = sysfs.NewDigitalPin(i)
		if err = muxPin(pin, "gpio"); err != nil {
			return
		}

		err := o.digitalPins[i].Export()
		if err != nil {
			return nil, err
		}
	}
	if err = o.digitalPins[i].Direction(dir); err != nil {
		return
	}
	return o.digitalPins[i], nil
}


// AnalogRead returns an analog value from specified pin
func (o *Adaptor) AnalogRead(pin string) (val int, err error) {
	analogPin, err := o.translateAnalogPin(pin)
	if err != nil {
		return
	}
	fi, err := sysfs.OpenFile(fmt.Sprintf("%v/%v", o.analogPath, analogPin), os.O_RDONLY, 0644)
	defer fi.Close()

	if err != nil {
		return
	}

	var ouf = make([]byte, 1024)
	_, err = fi.Read(ouf)
	if err != nil {
		return
	}

	val, _ = strconv.Atoi(strings.Split(string(ouf), "\n")[0])
	return
}

// GetConnection returns a connection to a device on a specified bus.
// Valid bus number is either 0 or 2 which corresponds to /dev/i2c-0 or /dev/i2c-2.
func (o *Adaptor) GetConnection(address int, bus int) (connection i2c.Connection, err error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if (bus != 0) && (bus != 2) {
		return nil, fmt.Errorf("bus number %d out of range", bus)
	}
	if o.i2cBuses[bus] == nil {
		o.i2cBuses[bus], err = sysfs.NewI2cDevice(fmt.Sprintf("/dev/i2c-%d", bus))
	}
	return i2c.NewConnection(o.i2cBuses[bus], address), err
}

// GetDefaultBus returns the default i2c bus for this platform
func (o *Adaptor) GetDefaultBus() int {
	return 1
}

// GetSpiConnection returns an spi connection to a device on a specified bus.
// Valid bus number is [0..1] which corresponds to /dev/spidev0.0 through /dev/spidev0.1.
func (o *Adaptor) GetSpiConnection(busNum, chipNum, mode, bits int, maxSpeed int64) (connection spi.Connection, err error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if (busNum < 0) || (busNum > 1) {
		return nil, fmt.Errorf("bus number %d out of range", busNum)
	}

	if o.spiBuses[busNum] == nil {
		o.spiBuses[busNum], err = spi.GetSpiConnection(busNum, chipNum, mode, bits, maxSpeed)
	}

	return o.spiBuses[busNum], err
}

// GetSpiDefaultBus returns the default spi bus for this platform.
func (o *Adaptor) GetSpiDefaultBus() int {
	return o.spiDefaultBus
}

// GetSpiDefaultChip returns the default spi chip for this platform.
func (o *Adaptor) GetSpiDefaultChip() int {
	return o.spiDefaultChip
}

// GetSpiDefaultMode returns the default spi mode for this platform.
func (o *Adaptor) GetSpiDefaultMode() int {
	return o.spiDefaultMode
}

// GetSpiDefaultBits returns the default spi number of bits for this platform.
func (o *Adaptor) GetSpiDefaultBits() int {
	return 8
}

// GetSpiDefaultMaxSpeed returns the default spi bus for this platform.
func (o *Adaptor) GetSpiDefaultMaxSpeed() int64 {
	return o.spiDefaultMaxSpeed
}

// translatePin converts digital pin name to pin position
func (o *Adaptor) translatePin(pin string) (value int, err error) {
	if val, ok := o.pinMap[pin]; ok {
		value = val
	} else {
		err = errors.New("not a valid pin")
	}
	return
}

// translateAnalogPin converts analog pin name to pin position
func (o *Adaptor) translateAnalogPin(pin string) (value string, err error) {
	if val, ok := o.analogPinMap[pin]; ok {
		value = val
	} else {
		err = errors.New("not a valid analog pin")
	}
	return
}

func muxPin(pin, cmd string) error {
	path := fmt.Sprintf("/sys/devices/platform/ocp/ocp:%s_pinmux/state", pin)
	fi, e := sysfs.OpenFile(path, os.O_WRONLY, 0666)
	defer fi.Close()
	if e != nil {
		return e
	}
	_, e = fi.WriteString(cmd)
	return e
}
