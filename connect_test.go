package apns

import "testing"

func TestTlsConnect(t *testing.T) {
	tlsConn := TlsConn{
		cert_file: "./test.pem",
		key_file:  "./test.pem",
		is_dev:    false,
	}

	if err := tlsConn.Connect(); err != nil {
		t.Errorf("connect to apns failed: %v", err)
	}

	if err := tlsConn.Reconnect(); err != nil {
		t.Errorf("reconnect to apns failed: %v", err)
	}

	if err := tlsConn.Close(); err != nil {
		t.Errorf("close connection failed: %v", err)
	}
}

func TestTlsConnectDev(t *testing.T) {
	tlsConn := TlsConn{
		cert_file: "./test.pem",
		key_file:  "./test.pem",
		is_dev:    true,
	}

	if err := tlsConn.Connect(); err != nil {
		t.Errorf("connect to apns failed: %v", err)
	}

	if err := tlsConn.Close(); err != nil {
		t.Errorf("close connection failed: %v", err)
	}
}
