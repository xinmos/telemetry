package cisco_telemetry_mdt

import (
	"time"

	"telemetry/internal"
)

const (
	// Maximum telemetry payload size (in bytes) to accept for GRPC dialout transport
	tcpMaxMsgLen uint32 = 1024 * 1024
)

const (
	GBPVALUE  = "ValueByType"
	GBPFIELDS = "fields"
	GBPNAME   = "name"
	Nexus     = "NX-OS"
)

const defaultKeepaliveMinTime = internal.Duration(time.Second * 300)
