package apns

const (
	//max payload in apple document
	MAX_PAYLOAD_SIZE = 2048
	//max valid token size
	TOKEN_SIZE = 64
	//length of error message
	ERROR_MESSAGE_LENGTH = 6
)

type Message struct {
	Token      string
	BinToken   []byte
	Payload    []byte
	ExpireTime uint32
	Priority   uint8
}

type ApnsError struct {
	ErrorCode uint8
	MessageId uint32
}
