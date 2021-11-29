package main

import (
	"crypto/tls"

	"go.ntppool.org/monitor/api/ca"
	"go.ntppool.org/monitor/api/pb"
	"google.golang.org/grpc/credentials"

	"google.golang.org/grpc"
)

func grpcConn() (*grpc.ClientConn, error) {

	capool, err := ca.CAPool()
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            capool,
	}
	conn, err := grpc.Dial("localhost:8000", grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		return nil, err
	}
	// defer conn.Close()
	return conn, err
}

func grpcClient() (pb.MonitorClient, error) {
	conn, err := grpcConn()
	if err != nil {
		return nil, err
	}

	client := pb.NewMonitorClient(conn)
	return client, err
}
