package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
	"os"
	"strconv"
	"time"

	flv "github.com/zhangpeihao/goflv"
	rtmp "seiPush/gortmp"
)

const (
	programName = "RtmpPublisher"
	version     = "0.0.1"
)

var (
	url         *string = flag.String("URL", "rtmp://domain/stage", "The rtmp url to connect.")
	streamName  *string = flag.String("Stream", "StreamKey", "Stream name to play.")
	flvFileName *string = flag.String("FLV", "./clock_av.flv", "FLV file to publishs.")
)

type TestOutboundConnHandler struct {
}

var obConn rtmp.OutboundConn
var createStreamChan chan rtmp.OutboundStream
var videoDataSize int64
var audioDataSize int64
var flvFile *flv.File

var status uint

func (handler *TestOutboundConnHandler) OnStatus(conn rtmp.OutboundConn) {
	var err error
	if obConn == nil {
		return
	}
	status, err = obConn.Status()
	fmt.Printf("@@@@@@@@@@@@@status: %d, err: %v\n", status, err)
}

func (handler *TestOutboundConnHandler) OnClosed(conn rtmp.Conn) {
	fmt.Printf("@@@@@@@@@@@@@Closed\n")
}

func (handler *TestOutboundConnHandler) OnReceived(conn rtmp.Conn, message *rtmp.Message) {
}

func (handler *TestOutboundConnHandler) OnReceivedRtmpCommand(conn rtmp.Conn, command *rtmp.Command) {
	fmt.Printf("ReceviedRtmpCommand: %+v\n", command)
}

func (handler *TestOutboundConnHandler) OnStreamCreated(conn rtmp.OutboundConn, stream rtmp.OutboundStream) {
	fmt.Printf("Stream created: %d\n", stream.ID())
	createStreamChan <- stream
}
func (handler *TestOutboundConnHandler) OnPlayStart(stream rtmp.OutboundStream) {

}
func (handler *TestOutboundConnHandler) OnPublishStart(stream rtmp.OutboundStream) {
	// Set chunk buffer size
	go publish(stream)
}

func publish(stream rtmp.OutboundStream) {
	fmt.Println("1")
	var err error
	flvFile, err = flv.OpenFile(*flvFileName)
	if err != nil {
		fmt.Println("Open FLV dump file error:", err)
		return
	}
	fmt.Println("2")
	defer flvFile.Close()
	startTs := uint32(0)
	startAt := time.Now().UnixNano()
	preTs := uint32(0)
	fmt.Println("3")
	for status == rtmp.OUTBOUND_CONN_STATUS_CREATE_STREAM_OK {
		if flvFile.IsFinished() {
			fmt.Println("@@@@@@@@@@@@@@File finished")
			flvFile.LoopBack()
			startAt = time.Now().UnixNano()
			startTs = uint32(0)
			preTs = uint32(0)
		}
		header, data, err := flvFile.ReadTag()
		if err != nil {
			fmt.Println("flvFile.ReadTag() error:", err)
			break
		}

		header, data = addSeiNalu(header, data)

		switch header.TagType {
		case flv.VIDEO_TAG:
			videoDataSize += int64(len(data))
		case flv.AUDIO_TAG:
			audioDataSize += int64(len(data))
		}

		if startTs == uint32(0) {
			startTs = header.Timestamp
		}
		diff1 := uint32(0)
		//		deltaTs := uint32(0)
		if header.Timestamp > startTs {
			diff1 = header.Timestamp - startTs
		} else {
			fmt.Printf("@@@@@@@@@@@@@@diff1 header(%+v), startTs: %d\n", header, startTs)
		}
		if diff1 > preTs {
			//			deltaTs = diff1 - preTs
			preTs = diff1
		}
		fmt.Printf("@@@@@@@@@@@@@@diff1 header(%+v), startTs: %d\n", header, startTs)
		if err = stream.PublishData(header.TagType, data, diff1); err != nil {
			fmt.Println("PublishData() error:", err)
			break
		}
		diff2 := uint32((time.Now().UnixNano() - startAt) / 1000000)
		//		fmt.Printf("diff1: %d, diff2: %d\n", diff1, diff2)
		if diff1 > diff2+100 {
			//			fmt.Printf("header.Timestamp: %d, now: %d\n", header.Timestamp, time.Now().UnixNano())
			time.Sleep(time.Millisecond * time.Duration(diff1-diff2))
		}
	}
}

func addSeiNalu(header *flv.TagHeader, data []byte) (*flv.TagHeader, []byte) {
	//frameType := data[0] >> 4
	//if frameType != 1 {
	//	return header, data
	//}
	codecId := data[0] & 0b00001111
	if codecId != 7 {
		return header, data
	}
	avcPacketType := data[1]
	if avcPacketType != 1 {
		return header, data
	}

	//fmt.Printf("%x\n", data[:100])

	seiNalu := buildSeiNalu()
	//fmt.Printf("%x\n", seiNalu)
	newData := bytes.Join([][]byte{data[:5], seiNalu, data[5:]}, []byte{})
	//fmt.Printf("%x\n", newData[:100])

	header.DataSize = header.DataSize + uint32(len(seiNalu))
	return header, newData
}

func buildSeiNalu() []byte {
	unixTime := time.Now().UnixNano() / 1e6
	//unixTimeBytes := Int64ToBytes(unixTime)
	unixTimeStr := strconv.FormatInt(unixTime, 10)
	unixTimeBytes := []byte(unixTimeStr)
	seiBytes := bytes.Join([][]byte{[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, unixTimeBytes}, []byte(""))

	bs := codec.NewBitStream(seiBytes)

	sei := &codec.SEI{}
	sei.PayloadType = 0x05
	sei.PayloadSize = uint16(len(seiBytes))
	sei.Sei_payload = new(codec.UserDataUnregistered)
	sei.Sei_payload.Read(sei.PayloadSize, bs)

	bsw := codec.NewBitStreamWriter(4096)
	seiPayload := sei.Encode(bsw)
	seiNaluPayload := bytes.Join([][]byte{[]byte{0x06}, seiPayload, []byte{0x80}}, []byte{})
	return bytes.Join([][]byte{Int64ToBytes(int64(len(seiNaluPayload)))[4:], seiNaluPayload}, []byte{})
}

func Int64ToBytes(i int64) []byte {
	var buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i))
	return buf
}

func BytesToInt64(buf []byte) int64 {
	return int64(binary.BigEndian.Uint64(buf))
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s version[%s]\r\nUsage: %s [OPTIONS]\r\n", programName, version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	createStreamChan = make(chan rtmp.OutboundStream)
	testHandler := &TestOutboundConnHandler{}
	fmt.Println("to dial")
	fmt.Println("a")
	var err error
	obConn, err = rtmp.Dial(*url, testHandler, 100)
	if err != nil {
		fmt.Println("Dial error", err)
		os.Exit(-1)
	}
	fmt.Println("b")
	defer obConn.Close()
	fmt.Println("to connect")
	err = obConn.Connect()
	if err != nil {
		fmt.Printf("Connect error: %s", err.Error())
		os.Exit(-1)
	}
	fmt.Println("c")
	for {
		select {
		case stream := <-createStreamChan:
			// Publish
			stream.Attach(testHandler)
			err = stream.Publish(*streamName, "live")
			if err != nil {
				fmt.Printf("Publish error: %s", err.Error())
				os.Exit(-1)
			}

		case <-time.After(1 * time.Second):
			fmt.Printf("Audio size: %d bytes; Vedio size: %d bytes\n", audioDataSize, videoDataSize)
		}
	}
}
