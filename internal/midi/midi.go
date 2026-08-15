package midi

type Destination struct {
	Index        int
	UniqueID     int32
	Name         string
	Manufacturer string
	Model        string
	Offline      bool
	MaxSysExRate int32
}

type Output interface {
	Send(message []byte) error
	Close() error
}
