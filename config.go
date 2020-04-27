package sleepy

type Config struct {
	FragmentAbove                uint
	FragmentSize                 uint
	MaxFragments                 uint
	MaxPacketSize                uint
	PacketHeaderSize             uint
	SentPacketBufferSize         uint
	RecvPacketBufferSize         uint
	FragmentReassemblyBufferSize uint

	RTTSmoothingFactor        float64
	PacketLossSmoothingFactor float64
	BandwidthSmoothingFactor  float64
}

func NewConfig() *Config {
	return &Config{
		FragmentAbove:                1024,
		FragmentSize:                 1024,
		MaxFragments:                 16,
		MaxPacketSize:                16 * 1024,
		PacketHeaderSize:             20,
		SentPacketBufferSize:         256,
		RecvPacketBufferSize:         256,
		FragmentReassemblyBufferSize: 256,

		RTTSmoothingFactor:        .0025,
		PacketLossSmoothingFactor: .1,
		BandwidthSmoothingFactor:  .1,
	}
}
