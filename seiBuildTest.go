package main

import (
	"bytes"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
)

func main() {
	payloadData := "TENCENT SEI TEST"
	seiBytes := bytes.Join([][]byte{{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, []byte(payloadData)}, []byte(""))

	bs := codec.NewBitStream(seiBytes)

	sei := &codec.SEI{}
	sei.PayloadType = 0x05
	sei.PayloadSize = uint16(len(seiBytes))
	sei.Sei_payload = new(codec.UserDataUnregistered)
	sei.Sei_payload.Read(sei.PayloadSize, bs)

	bsw := codec.NewBitStreamWriter(4096)
	seiPayload := sei.Encode(bsw)
	seiNaluPayload := bytes.Join([][]byte{{0x06}, seiPayload, {0x80}}, []byte{})
	fmt.Printf("%x", seiNaluPayload)
}
