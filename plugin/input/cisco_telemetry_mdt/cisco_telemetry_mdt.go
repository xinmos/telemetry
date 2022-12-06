package cisco_telemetry_mdt

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/cisco-ie/nx-telemetry-proto/mdt_dialout"
	"github.com/cisco-ie/nx-telemetry-proto/telemetry_bis"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"

	"telemetry/models"
	interTLS "telemetry/plugin/common/tls"
)

type GRPCEnforcementPolicy struct {
	PermitKeepaliveWithoutCalls bool     `json:"permit_keepalive_without_calls"`
	KeepaliveMinTime            Duration `json:"keepalive_minimum_time"`
}

type CiscoTelemetryMDT struct {
	// Common configuration
	Transport         string                `json:"transport"`
	ServiceAddress    string                `json:"service_address"`
	MaxMsgSize        int                   `json:"max_msg_size"`
	EnforcementPolicy GRPCEnforcementPolicy `json:"grpc_enforcement_policy"`

	log *logrus.Entry

	// GRPC TLS settings
	interTLS.ServerConfig

	grpcServer *grpc.Server
	listener   net.Listener

	mutex sync.Mutex
	acc   models.Accumulator
	wg    sync.WaitGroup

	// Though unused in the code, required by protoc-gen-go-grpc to maintain compatibility
	mdtdialout.UnimplementedGRPCMdtDialoutServer
}

func NewCiscoTelemetryMDT() *CiscoTelemetryMDT {
	return &CiscoTelemetryMDT{
		log: models.NewLogger("inputs.cisco_telemetry_mdt"),
	}
}

func (c *CiscoTelemetryMDT) Start(acc models.Accumulator) error {
	var err error
	c.acc = acc
	c.listener, err = net.Listen("tcp", c.ServiceAddress)
	if err != nil {
		return err
	}

	switch c.Transport {
	case "tcp":
		// TCP dialout server accept routine
		c.wg.Add(1)
		go func() {
			c.acceptTCPClients()
			c.wg.Done()
		}()
	case "grpc":
		var opts []grpc.ServerOption
		tlsConfig, err := c.ServerConfig.TLSConfig()
		if err != nil {
			_ = c.listener.Close()
			return err
		} else if tlsConfig != nil {
			opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		}

		if c.MaxMsgSize > 0 {
			opts = append(opts, grpc.MaxRecvMsgSize(c.MaxMsgSize))
		}

		if c.EnforcementPolicy.PermitKeepaliveWithoutCalls ||
			(c.EnforcementPolicy.KeepaliveMinTime != 0 && c.EnforcementPolicy.KeepaliveMinTime != defaultKeepaliveMinTime) {
			// Only set if either parameter does not match defaults
			opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             time.Duration(c.EnforcementPolicy.KeepaliveMinTime),
				PermitWithoutStream: c.EnforcementPolicy.PermitKeepaliveWithoutCalls,
			}))
		}

		c.grpcServer = grpc.NewServer(opts...)
		mdtdialout.RegisterGRPCMdtDialoutServer(c.grpcServer, c)

		c.wg.Add(1)
		go func() {
			if err := c.grpcServer.Serve(c.listener); err != nil {
				c.log.Errorf("serving GRPC server failed: %v", err)
			}
			c.wg.Done()
		}()
	default:
		_ = c.listener.Close()
		return fmt.Errorf("invalid Cisco MDT transport: %s", c.Transport)
	}

	return nil
}

func (c *CiscoTelemetryMDT) acceptTCPClients() {
	var mutex sync.Mutex
	clients := make(map[net.Conn]struct{})

	for {
		conn, err := c.listener.Accept()
		if neterr, ok := err.(*net.OpError); ok && (neterr.Timeout() || neterr.Temporary()) {
			continue
		} else if err != nil {
			break
		}

		mutex.Lock()
		clients[conn] = struct{}{}
		mutex.Unlock()

		c.wg.Add(1)
		go func() {
			c.log.Infof("Accepted Cisco MDT TCP dialout connection from %s", conn.RemoteAddr())
			if err := c.handleTCPClient(conn); err != nil {
				c.log.Errorf("handle tcp client error: %v", err)
			}
			c.log.Infof("Closed Cisco MDT TCP dialout connection from %s", conn.RemoteAddr())

			mutex.Lock()
			delete(clients, conn)
			mutex.Unlock()

			if err := conn.Close(); err != nil {
				c.log.Errorf("closing connection failed: %v", err)
			}
			c.wg.Done()
		}()
	}

	// Close all remaining client connections
	mutex.Lock()
	for client := range clients {
		if err := client.Close(); err != nil {
			c.log.Errorf("Failed to close TCP dialout client: %v", err)
		}
	}
	mutex.Unlock()
}

func (c *CiscoTelemetryMDT) handleTCPClient(conn net.Conn) error {
	// TCP Dialout telemetry framing header
	var hdr struct {
		MsgType       uint16
		MsgEncap      uint16
		MsgHdrVersion uint16
		MsgFlags      uint16
		MsgLen        uint32
	}

	var payload bytes.Buffer
	sourceIp := conn.RemoteAddr().String()

	for {
		// Read and validate dialout telemetry header
		if err := binary.Read(conn, binary.BigEndian, &hdr); err != nil {
			return err
		}

		maxMsgSize := tcpMaxMsgLen
		if c.MaxMsgSize > 0 {
			maxMsgSize = uint32(c.MaxMsgSize)
		}

		if hdr.MsgLen > maxMsgSize {
			return fmt.Errorf("dialout packet too long: %v", hdr.MsgLen)
		} else if hdr.MsgFlags != 0 {
			return fmt.Errorf("invalid dialout flags: %v", hdr.MsgFlags)
		}

		// Read and handle telemetry packet
		payload.Reset()
		if size, err := payload.ReadFrom(io.LimitReader(conn, int64(hdr.MsgLen))); size != int64(hdr.MsgLen) {
			if err != nil {
				return err
			}
			return fmt.Errorf("TCP dialout premature EOF")
		}

		c.handleTelemetry(payload.Bytes(), sourceIp)
	}
}

// MdtDialout RPC server method for grpc-dialout transport
func (c *CiscoTelemetryMDT) MdtDialout(stream mdtdialout.GRPCMdtDialout_MdtDialoutServer) error {
	peerInCtx, peerOK := peer.FromContext(stream.Context())
	if peerOK {
		c.log.Infof("Accepted Cisco MDT GRPC dialout connection from %s", peerInCtx.Addr)
	}

	var chunkBuffer bytes.Buffer
	sourceIP := peerInCtx.Addr.String()

	for {
		packet, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				c.log.Errorf("GRPC dialout receive error: %v", err)
			}
			break
		}

		if len(packet.Data) == 0 && len(packet.Errors) != 0 {
			c.log.Errorf("GRPC dialout error: %s", packet.Errors)
			break
		}

		// Reassemble chunked telemetry data received from NX-OS
		if packet.TotalSize == 0 {
			c.handleTelemetry(packet.Data, sourceIP)
		} else if int(packet.TotalSize) <= c.MaxMsgSize {
			if _, err := chunkBuffer.Write(packet.Data); err != nil {
				c.log.Errorf("writing packet %q failed: %v", packet.Data, err)
			}
			if chunkBuffer.Len() >= int(packet.TotalSize) {
				c.handleTelemetry(chunkBuffer.Bytes(), sourceIP)
				chunkBuffer.Reset()
			}
		} else {
			c.log.Errorf("dropped too large packet: %dB > %dB", packet.TotalSize, c.MaxMsgSize)
		}
	}

	if peerOK {
		c.log.Infof("Closed Cisco MDT GRPC dialout connection from %s", peerInCtx.Addr)
	}

	return nil
}

func (c *CiscoTelemetryMDT) Stop() {
	if c.grpcServer != nil {
		// Stop server and terminate all running dialout routinesb
		//nolint:errcheck,revive // we cannot do anything if the stopping fails
		c.grpcServer.Stop()
	}
	if c.listener != nil {
		//nolint:errcheck,revive // we cannot do anything if the closing fails
		_ = c.listener.Close()
	}
	c.wg.Wait()
}

func (c *CiscoTelemetryMDT) Gather(_ models.Accumulator) error {
	return nil
}

func (c *CiscoTelemetryMDT) handleTelemetry(data []byte, sourceIP string) {
	msg := &telemetry_bis.Telemetry{}
	err := proto.Unmarshal(data, msg)
	if err != nil {
		c.log.Errorf("failed to decode: %v", err)
		return
	}

	m := NewCiscoTelemetryMetric(sourceIP)
	gbpkv, err := json.Marshal(msg)
	if err != nil {
		c.log.Errorf("Gpbkv Parse Failure")
	}

	var v any
	err = json.Unmarshal(gbpkv, &v)
	if err != nil {
		c.log.Infoln("unmarshal json err: ", err)
	}

	telemetryData := v.(map[string]any)
	for k, v := range telemetryData {
		if k != "data_gpbkv" {
			m.parseTelemetry(k, v)
		} else {
			err = m.parseRow(v)
			if err != nil {
				c.log.Errorf("parse row data error")
			}
		}
	}

	c.acc.AddMetric(m)
}

func (c *CiscoTelemetryMDT) ParseConfig(cfg map[string]any) error {
	tmp, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(tmp, c)
	if err != nil {
		return fmt.Errorf("[cisco_telemetry_mdt] config error: %v", err)
	}
	return nil
}
