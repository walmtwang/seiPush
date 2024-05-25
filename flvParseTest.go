package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-flv"
	"net/http"
	"os"
	"strconv"
	"time"
)

type JacoSei struct {
	Ts string `json:"ts"`
}

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
	fmt.Printf("http status:%v\n", resp.StatusCode)
	if resp.StatusCode != 200 {
		return
	}

	flvReader := flv.CreateFlvReader()
	flvReader.OnFrame = func(cid codec.CodecID, frame []byte, pts, dts uint32) {
		//fmt.Printf("frame:%x\n", frame[:100])
		//fmt.Printf("cid:%v, nalutype:%v\n", cid, frame[4])
		isSei := false
		nextIndex := 0
		if cid == codec.CODECID_VIDEO_H264 && frame[4] == 0x06 {
			isSei = true
			nextIndex = 5
		}
		if cid == codec.CODECID_VIDEO_H265 && frame[4] == 0x4e && frame[5] == 01 {
			isSei = true
			nextIndex = 6
		}
		if !isSei {
			return
		}

		nowTime := time.Now().UnixNano() / 1e6
		//fmt.Printf("data:%x\n", frame)
		payloadType := 0
		for frame[nextIndex] == 0xff {
			payloadType += int(frame[nextIndex])
			nextIndex++
		}
		payloadType += int(frame[nextIndex])
		nextIndex++

		payloadSize := 0
		for frame[nextIndex] == 0xff {
			payloadSize += int(frame[nextIndex])
			nextIndex++
		}
		payloadSize += int(frame[nextIndex])
		nextIndex++

		payloadInfo := string(frame[nextIndex : nextIndex+payloadSize])
		fmt.Printf("nowTime:%v, payloadType:%v, payloadSize:%v, payloadInfo:%v\n", nowTime, payloadType, payloadSize, payloadInfo)

		sei := new(JacoSei)
		if err := json.Unmarshal(frame[nextIndex:nextIndex+payloadSize], sei); err != nil {
			fmt.Printf("json unmarshal failed, err:%v\n", err)
			return
		}
		tsTime, err := strconv.ParseInt(sei.Ts, 10, 64)
		if err != nil {
			fmt.Printf("strconv ParseInt failed, err:%v\n", err)
			return
		}
		fmt.Printf("nowTime:%v,sei ts:%v, gap:%v\n", nowTime, tsTime, nowTime-tsTime)

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
