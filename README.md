##apns
lib for apns

##install
go get -u github.com/astrophor/apns

##example
go code:

```
package main

import (
	"fmt"

	"github.com/astrophor/apns"
)

func main() {
	apnsHandler, err := apns.Connect("./test.pem", "./test.pem", false)
	if err != nil {
		panic("connect to apns failed: " + err.Error())
	}
	fmt.Println("connect to apns succ")

	text := `{"aps":{"alert":"hello, world"}}`
	msg := apns.Message{
		Token:   "xxx",
		Payload: []byte(text),
	}

	if err := apnsHandler.Send(msg); err != nil {
		fmt.Println(err)
	}

	apnsHandler.Close()
}
```

