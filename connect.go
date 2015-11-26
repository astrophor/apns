package apns

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	//apns host for production environment
	GATEWAY_HOST = "gateway.push.apple.com"
	//apns port for production environment
	GATEWAY_PORT = "2195"
	//apns host for development environment
	GATEWAY_DEV_HOST = "gateway.sandbox.push.apple.com"
	//apns port for development environment
	GATEWAY_DEV_PORT = "2195"
	//socket read time out
	READ_TIME_OUT = 5
	//socket write time out
	WRITE_TIME_OUT = 3
)

type TlsConn struct {
	socket    net.Conn
	cert_file string
	key_file  string
	is_dev    bool
	mu        sync.RWMutex
}

func (t *TlsConn) Connect() error {
	addr := fmt.Sprintf("%v:%v", GATEWAY_HOST, GATEWAY_PORT)
	if t.is_dev {
		addr = fmt.Sprintf("%v:%v", GATEWAY_DEV_HOST, GATEWAY_DEV_PORT)
	}

	tcp_socket, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connect to %v failed: %v", addr, err)
	}

	cert, err := tls.LoadX509KeyPair(t.cert_file, t.key_file)
	if err != nil {
		return fmt.Errorf("load key pair failed: %v", err)
	}

	tls_conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   GATEWAY_HOST,
	}
	if t.is_dev {
		tls_conf.ServerName = GATEWAY_DEV_HOST
	}

	tls_socket := tls.Client(tcp_socket, tls_conf)
	err = tls_socket.Handshake()
	if err != nil {
		return fmt.Errorf("tls hand shake to %v failed: %v", addr, err)
	}

	log.Printf("connect to %v succ", addr)

	t.mu.Lock()
	t.socket = tls_socket
	t.mu.Unlock()

	return nil
}

func (t *TlsConn) Reconnect() error {
	addr := fmt.Sprintf("%v:%v", GATEWAY_HOST, GATEWAY_PORT)
	if t.is_dev {
		addr = fmt.Sprintf("%v:%v", GATEWAY_DEV_HOST, GATEWAY_DEV_PORT)
	}

	tcp_socket, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connect to %v failed: %v", addr, err)
	}

	cert, err := tls.LoadX509KeyPair(t.cert_file, t.key_file)
	if err != nil {
		return fmt.Errorf("load key pair failed: %v", err)
	}

	tls_conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   GATEWAY_HOST,
	}
	if t.is_dev {
		tls_conf.ServerName = GATEWAY_DEV_HOST
	}

	tls_socket := tls.Client(tcp_socket, tls_conf)
	err = tls_socket.Handshake()
	if err != nil {
		return fmt.Errorf("tls hand shake to %v failed: %v", addr, err)
	}

	log.Printf("connect to %v succ", addr)

	t.mu.Lock()
	t.socket.Close()
	t.socket = tls_socket
	t.mu.Unlock()

	return nil
}

func (t *TlsConn) Read(b []byte) (int, error) {
	t.mu.RLock()

	t.socket.SetReadDeadline(time.Now().Add(READ_TIME_OUT * time.Second))
	read_len, err := t.socket.Read(b)

	t.mu.RUnlock()

	return read_len, err
}

func (t *TlsConn) Write(b []byte) (int, error) {
	t.mu.RLock()

	t.socket.SetWriteDeadline(time.Now().Add(WRITE_TIME_OUT * time.Second))
	write_len, err := t.socket.Write(b)

	t.mu.RUnlock()

	return write_len, err
}

func (t *TlsConn) Close() error {
	t.mu.Lock()

	err := t.socket.Close()

	t.mu.Unlock()

	return err
}
