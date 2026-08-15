package midi

type VirtualDevice interface {
	Name() string
	Received() <-chan []byte
	Send(message []byte) error
	Close() error
}
