package apns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	ADDR        = ":8080"
	read_num    = int32(0)
	delta       = 2
	connect_num = 0
)

func StartApnsServer() {
	listener, err := net.Listen("tcp", ADDR)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept failed: ", err)
			continue
		}

		go ApnsServerHandler(conn)
	}
}

func ReadOnePacket(conn net.Conn) error {
	var head [5]byte
	var body [4096]byte

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Read(head[:]); err != nil {
		//log.Println("read head failed: ", err)
		return err
	}

	body_len := binary.BigEndian.Uint32(head[1:])
	if body_len > 4096 {
		return fmt.Errorf("body too long: %v > 4096", body_len)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(body[:body_len]); err != nil {
		//log.Println("read body failed: ", err)
		return err
	}

	return nil
}

func WriteErrorMsg(conn net.Conn) error {
	buffer := new(bytes.Buffer)
	binary.Write(buffer, binary.BigEndian, uint8(8))
	binary.Write(buffer, binary.BigEndian, uint8(10))
	binary.Write(buffer, binary.BigEndian, uint32(2))

	if write_len, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	} else if write_len != 6 {
		return fmt.Errorf("wrong length %v != 6", write_len)
	}

	return nil
}

func ApnsServerHandler(conn net.Conn) {
	connect_num += 1

	for idx := 0; idx < delta; idx++ {
		if err := ReadOnePacket(conn); err != nil {
			fmt.Println(err)
			return
		}
		atomic.AddInt32(&read_num, 1)
	}

	if err := WriteErrorMsg(conn); err != nil {
		log.Println(err)
		return
	}

	delta *= 2

	conn.Close()
}

type TestConn struct {
	conn net.Conn
	mu   sync.RWMutex
}

func (t *TestConn) Connect() error {
	conn, err := net.Dial("tcp", ADDR)
	if err != nil {
		return err
	}
	log.Println("connect succ")

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	return nil
}

func (t *TestConn) Reconnect() error {
	conn, err := net.Dial("tcp", ADDR)
	if err != nil {
		return err
	}
	log.Println("reconnect succ")

	t.mu.Lock()
	t.conn.Close()
	t.conn = conn
	t.mu.Unlock()

	return nil
}

func (t *TestConn) Read(p []byte) (n int, err error) {
	t.mu.RLock()

	t.conn.SetReadDeadline(time.Now().Add(READ_TIME_OUT * time.Second))
	read_len, err := t.conn.Read(p)

	t.mu.RUnlock()

	return read_len, err
}

func (t *TestConn) Write(p []byte) (n int, err error) {
	t.mu.RLock()

	t.conn.SetWriteDeadline(time.Now().Add(WRITE_TIME_OUT * time.Second))
	write_len, err := t.conn.Write(p)

	t.mu.RUnlock()

	return write_len, err
}

func (t *TestConn) Close() error {
	t.mu.Lock()

	err := t.conn.Close()

	t.mu.Unlock()

	return err
}

func TestError(t *testing.T) {
	var c TestConn
	if err := c.Connect(); err != nil {
		t.Error(err)
	}

	apnsHandler := Apns{
		connector: &c,
	}
	apnsHandler.Init()

	text := `{"aps":{"alert":"hello, world"}}`
	msg := Message{
		Token:      "xxx",
		Payload:    []byte(text),
		ExpireTime: 3600,
		Priority:   10,
	}

	for idx := 0; idx < 4; idx++ {
		if err := apnsHandler.Send(msg); err != nil {
			t.Error(err)
		}
	}

	time.Sleep(13 * time.Second)

	apnsHandler.Close()

	if read_num != 4 {
		t.Errorf("get wrong msg number: %v != 4", read_num)
	}

	if connect_num != 2 {
		t.Errorf("get wrong reconnect number: %v != 2", connect_num)
	}
}

func TestMain(m *testing.M) {
	go StartApnsServer()

	time.Sleep(time.Second)
	ret := m.Run()

	os.Exit(ret)
}
