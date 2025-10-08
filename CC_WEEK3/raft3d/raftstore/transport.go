package raftstore

import (
	"net"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
)

func NewRaftTransport(
	bindAddr string,
	maxPool int,
	timeout time.Duration,
	logOutput hclog.Logger, // Changed to hclog.Logger
) (*raft.NetworkTransport, error) {
	addr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		return nil, err
	}
	
	return raft.NewTCPTransportWithLogger(
		bindAddr,
		addr,
		maxPool,
		timeout,
		logOutput, // Now matches hclog.Logger type
	)
}