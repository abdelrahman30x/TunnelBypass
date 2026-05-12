package network

// PrivateRoutingMode maps directly to what gets written into Xray routing rules.
type PrivateRoutingMode int

const (
	BlockPrivate PrivateRoutingMode = iota // geoip:private → block outbound
)

// NetworkProfile is kept as a small compatibility carrier for config generators.
type NetworkProfile struct{}

// PrivateRouting returns the routing policy to use for Xray-based transports.
func (p NetworkProfile) PrivateRouting() PrivateRoutingMode {
	return BlockPrivate
}
