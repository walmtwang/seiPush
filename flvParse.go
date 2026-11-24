package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-flv"
	"net/http"
	"os"
	"strconv"
	"time"
)
const (
	programName = "FlvParse"
	version     = "0.0.1"
)

var (
	url *string = flag.String("URL", "http://domain/stage/TestDelay20230426.flv", "The rtmp url to connect.")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s version[%s]\r\nUsage: %s [OPTIONS]\r\n", programName, version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	resp, err := http.Get(*url)
	if err != nil {
		fmt.Printf("http.Get err:%v\n", err)
		return
	}
	defer resp.Body.Close()

	flvReader := flv.CreateFlvReader()
	flvReader.OnFrame = func(cid codec.CodecID, frame []byte, pts, dts uint32) {
		//fmt.Printf("frame:%x\n", frame[:100])
		if cid != codec.CODECID_VIDEO_H264 {
			return
		}
		if frame[4] != 0x06 {
			return
		}
		if frame[5] != 0x05 {
			return
		}
		unixTimeBytes := frame[23 : 23+13]
		seiTime, _ := strconv.ParseInt(string(unixTimeBytes), 10, 64)
		nowTime := time.Now().UnixNano() / 1e6
		fmt.Printf("nowTime:%v,seiTime:%v,delay:%v\n", nowTime, seiTime, nowTime-seiTime)
	}

	cache := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(cache)
		if err != nil {
			fmt.Printf("resp.Body.Read err:%v\n", err)
			return
		}
		err = flvReader.Input(cache[0:n])
		if err != nil {
			fmt.Printf("http.Get err:%v\n", err)
			return
		}
	}

}

func BytesToInt64(buf []byte) int64 {
	return int64(binary.BigEndian.Uint64(buf))
}
