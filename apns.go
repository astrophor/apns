package apns

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	//number of payloads keeped for error handling(resent)
	MAX_PAYLOAD_CACHE_NUM = 10000
	//Notification head size
	NOTIFICATION_HEAD_SIZE = 1 + 4
	//write retry number
	RETRY_NUMBER = 2
	//write concurrent number
	CONCURRENT_NUMBER = 3
)

type ApnsConnector interface {
	Connect() error
	Reconnect() error
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

func Connect(cert_file, key_file string, is_dev bool) (Apns, error) {
	tlsConn := TlsConn{
		cert_file: cert_file,
		key_file:  key_file,
		is_dev:    is_dev,
	}

	if err := tlsConn.Connect(); err != nil {
		return Apns{}, err
	}

	apns := Apns{
		connector: &tlsConn,
	}
	apns.Init()

	return apns, nil
}

type Apns struct {
	//connector to apns, normally it's a socket, create this interface just for test
	connector ApnsConnector
	//channel that for send message
	sendChannel chan Message
	//id in notification format, maintain by lib inside
	payloadId uint32

	//cache message for error handling
	cachedMessages *list.List
	//cache message mutex
	cachedMsgLock *sync.Mutex
}

type cacheMessage struct {
	id  uint32
	msg Message
}

func (a *Apns) Init() {
	a.payloadId = uint32(1)
	a.sendChannel = make(chan Message, 100)
	a.cachedMessages = list.New()
	a.cachedMsgLock = new(sync.Mutex)

	for idx := 0; idx < CONCURRENT_NUMBER; idx++ {
		go a.sendHandler()
	}
	go a.errorHadnler()
}

func (a *Apns) Send(msg Message) error {
	if len(msg.Token) != TOKEN_SIZE {
		return fmt.Errorf("invalid token[%v]: length %v != %v", msg.Token, len(msg.Token), TOKEN_SIZE)
	}

	var err error
	if msg.BinToken, err = hex.DecodeString(msg.Token); err != nil {
		return fmt.Errorf("trans token to binary failed: %v", err)
	}

	if len(msg.Payload) > MAX_PAYLOAD_SIZE {
		return fmt.Errorf("payload size %v is more than expect %v", len(msg.Payload), MAX_PAYLOAD_SIZE)
	}

	a.sendChannel <- msg

	return nil
}

func (a *Apns) Close() error {
	close(a.sendChannel)
	time.Sleep(time.Second)

	return a.connector.Close()
}

func (a *Apns) sendHandler() {
	for {
		pmsg, ok := <-a.sendChannel
		if !ok {
			log.Println("send channel is closed, sendHandler goroutine is going to quit")
			return
		}

		a.sendMessage(pmsg)
	}
}

func (a *Apns) sendMessage(msg Message) {
	cachedMsg := cacheMessage{
		id:  a.payloadId,
		msg: msg,
	}

	atomic.AddUint32(&a.payloadId, uint32(1))

	//if send failed, retry
	a.sendMessageToApns(cachedMsg)

	//message was send, cache it for error handling
	a.addMessageToCacheQueue(cachedMsg)
}

func (a *Apns) packMessage(msg cacheMessage) []byte {
	itemBuf := new(bytes.Buffer)
	frameBuf := new(bytes.Buffer)

	binary.Write(itemBuf, binary.BigEndian, uint8(1))
	binary.Write(itemBuf, binary.BigEndian, uint16(TOKEN_SIZE/2))
	binary.Write(itemBuf, binary.BigEndian, msg.msg.BinToken)

	binary.Write(itemBuf, binary.BigEndian, uint8(2))
	binary.Write(itemBuf, binary.BigEndian, uint16(len(msg.msg.Payload)))
	binary.Write(itemBuf, binary.BigEndian, msg.msg.Payload)

	binary.Write(itemBuf, binary.BigEndian, uint8(3))
	binary.Write(itemBuf, binary.BigEndian, uint16(4))
	binary.Write(itemBuf, binary.BigEndian, msg.id)

	if msg.msg.ExpireTime > 0 {
		binary.Write(itemBuf, binary.BigEndian, uint8(4))
		binary.Write(itemBuf, binary.BigEndian, uint16(4))
		binary.Write(itemBuf, binary.BigEndian, msg.msg.ExpireTime)
	}

	if msg.msg.Priority > 0 {
		binary.Write(itemBuf, binary.BigEndian, uint8(5))
		binary.Write(itemBuf, binary.BigEndian, uint16(1))
		binary.Write(itemBuf, binary.BigEndian, msg.msg.Priority)
	}

	binary.Write(frameBuf, binary.BigEndian, uint8(2))
	binary.Write(frameBuf, binary.BigEndian, uint32(itemBuf.Len()))
	binary.Write(frameBuf, binary.BigEndian, itemBuf.Bytes())

	return frameBuf.Bytes()
}

func (a *Apns) sendMessageToApns(msg cacheMessage) {
	data := a.packMessage(msg)

	for idx := 0; idx < RETRY_NUMBER; idx++ {
		if sendLen, err := a.connector.Write(data); err != nil {
			log.Println("send message failed: ", err, " id ", msg.id)

			if is_timeout(err) {
				a.connector.Reconnect()
				continue
			}
		} else if sendLen != len(data) {
			log.Println("send whole message failed, send ", sendLen, " full length is ", len(data))
		} else {
			//log.Println("send msg succ, id ", msg.id)
			break
		}

		time.Sleep(8 * time.Second)
		log.Println("retry number: ", idx, " id: ", msg.id)
	}
}

func (a *Apns) addMessageToCacheQueue(msg cacheMessage) {
	a.cachedMsgLock.Lock()

	a.cachedMessages.PushBack(msg)
	if a.cachedMessages.Len() > MAX_PAYLOAD_CACHE_NUM {
		a.cachedMessages.Remove(a.cachedMessages.Front())
	}

	a.cachedMsgLock.Unlock()
}

func is_timeout(err error) bool {
	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	}

	return false
}

func is_closed(err error) bool {
	if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
		return true
	}

	return false
}

func (a *Apns) errorHadnler() {
	buffer := make([]byte, ERROR_MESSAGE_LENGTH, ERROR_MESSAGE_LENGTH)

	for {
		read_len, err := a.connector.Read(buffer)

		if read_len == ERROR_MESSAGE_LENGTH {
			error_msg := ApnsError{
				ErrorCode: uint8(buffer[1]),
				MessageId: binary.BigEndian.Uint32(buffer[2:]),
			}
			log.Printf("get an error from apple, error code: %v, message id: %v", error_msg.ErrorCode, error_msg.MessageId)

			a.connector.Reconnect()

			go a.resent(error_msg, true)
		} else if is_timeout(err) && read_len == 0 {
			continue
		} else if is_closed(err) {
			log.Println("socket closed by write")
			time.Sleep(time.Second)
			continue
		} else {
			log.Println("apns close without send error code, read length: ", read_len, " error info: ", err)

			a.connector.Reconnect()

			go a.resent(ApnsError{}, false)
		}
	}
}

func (a *Apns) resent(msg ApnsError, has_id bool) {
	a.cachedMsgLock.Lock()

	if has_id {
		unsent_list := list.New()
		for e := a.cachedMessages.Back(); e != nil; e = e.Prev() {
			obj := e.Value.(cacheMessage)
			if obj.id == msg.MessageId {
				log.Println("error msg token: ", obj.msg.Token)
				break
			}
			unsent_list.PushFront(obj.msg)
		}
		go a.ResentMessage(unsent_list)
	}
	a.cachedMessages.Init()

	a.cachedMsgLock.Unlock()
}

func (a *Apns) ResentMessage(unsent_list *list.List) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic in resent goroutine: ", err)
		}
	}()

	log.Println("resent length: ", unsent_list.Len())

	for e := unsent_list.Front(); e != nil; e = e.Next() {
		msg := e.Value.(Message)

		//这里有可能出现channel关闭了，但是仍旧往里面插入的情况
		//这个情况不太好处理，由于只有在程序退出时可能panic，没有太大影响
		//故通过上面捕获异常方式处理
		a.sendChannel <- msg
	}
}
