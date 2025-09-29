package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-flv"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	programName = "FlvParse"
	version     = "0.0.1"
)

var (
	httpUrl *string = flag.String("url", "", "flv url")
	ip      *string = flag.String("ip", "", "cdn ip")
	port    *string = flag.String("port", "80", "cdn port, default 80")
)

func main() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s version[%s]\nUsage: %s [OPTIONS]\n", programName, version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if *httpUrl == "" {
		fmt.Printf("httpUrl is empty\n")
		return
	}

	if *ip != "" && *port != "" {
		http.DefaultClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial(network, fmt.Sprintf("%s:%s", *ip, *port))
			},
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial(network, fmt.Sprintf("%s:%s", *ip, *port))
			},
		}
	}

	resp, err := http.Get(*httpUrl)
	if err != nil {
		fmt.Printf("http.Get err:%v\n", err)
		return
	}
	defer resp.Body.Close()

	var startTime int64 = 0
	var startPts uint32 = 0

	flvReader := flv.CreateFlvReader()
	flvReader.OnFrame = func(cid codec.CodecID, frame []byte, pts, dts uint32) {
		nowTime := time.Now().UnixNano() / 1e6
		if startTime == 0 {
			startTime = nowTime
			startPts = pts
		}
		if nowTime-startTime >= 10000 {
			bufferTime := int64(pts) - int64(startPts)
			downloadTime := nowTime - startTime
			fmt.Printf("download time:%v, buffer time:%v, flv buffer:%v\n", downloadTime, bufferTime, bufferTime-downloadTime)
			os.Exit(0)
		}
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
