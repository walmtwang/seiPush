package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-flv"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type PushSei struct {
	Ts     int64 `json:"ts"`
	RealTs int64 `json:"real_ts"`
}

const (
	programName = "FlvParse"
	version     = "0.0.1"
)

var (
	httpUrl *string = flag.String("url", "http://domain/stage/TestDelay20230426.flv", "The rtmp url to connect.")
	ip      *string = flag.String("ip", "1.1.1.1", "cdn ip")
	port    *int    = flag.Int("port", 80, "port")
)

type NetConn struct {
	QuicConn   quic.Connection
	QuicStream quic.Stream
}

func (netConn *NetConn) Read(b []byte) (n int, err error) {
	return netConn.QuicStream.Read(b)
}
func (netConn *NetConn) Write(b []byte) (n int, err error) {
	return netConn.QuicStream.Write(b)
}
func (netConn *NetConn) Close() error {
	return netConn.QuicStream.Close()
}
func (netConn *NetConn) LocalAddr() net.Addr {
	return netConn.QuicConn.LocalAddr()
}
func (netConn *NetConn) RemoteAddr() net.Addr {
	return netConn.QuicConn.RemoteAddr()
}
func (netConn *NetConn) SetDeadline(t time.Time) error {
	return netConn.QuicStream.SetDeadline(t)
}
func (netConn *NetConn) SetReadDeadline(t time.Time) error {
	return netConn.QuicStream.SetReadDeadline(t)
}
func (netConn *NetConn) SetWriteDeadline(t time.Time) error {
	return netConn.QuicStream.SetWriteDeadline(t)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s version[%s]\r\nUsage: %s [OPTIONS]\r\n", programName, version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	fmt.Printf("url:%v, ip:%v, port:%v\n", *httpUrl, *ip, *port)

	url2, err := url.Parse(*httpUrl)
	if err != nil {
		log.Fatalf("url.Parse failed, err:%v", err)
	}
	domain := strings.Split(url2.Host, ":")[0]

	roundTripper := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := quic.DialAddrContext(ctx,
				fmt.Sprintf("%s:%d", *ip, *port),
				&tls.Config{
					ServerName: domain,
					NextProtos: []string{"http over quic"},
				}, &quic.Config{
					Versions: []quic.VersionNumber{quic.VersionDraft29},
				})
			if err != nil {
				return nil, err
			}
			stream, err := conn.OpenStreamSync(ctx)
			var netConn = &NetConn{
				QuicConn:   conn,
				QuicStream: stream,
			}
			return netConn, err
		},
	}

	hclient := &http.Client{
		Transport: roundTripper,
	}
	resp, err := hclient.Get(*httpUrl)
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
		if payloadType != 100 {
			return
		}
		nextIndex++

		payloadSize := 0
		for frame[nextIndex] == 0xff {
			payloadSize += int(frame[nextIndex])
			nextIndex++
		}
		payloadSize += int(frame[nextIndex])
		nextIndex++

		//payloadInfo := string(frame[nextIndex : nextIndex+payloadSize])
		//fmt.Printf("payloadType:%v, payloadSize:%v, payloadInfo:%v\n", payloadType, payloadSize, payloadInfo)
		//fmt.Printf("payloadInfo:%v\n", frame[nextIndex : nextIndex+payloadSize])
		seiStruct := new(PushSei)
		err := json.Unmarshal(frame[nextIndex:nextIndex+payloadSize], seiStruct)
		if err != nil {
			fmt.Printf("json.Unmarshal err:%v\n", err)
			return
		}
		seiTime := seiStruct.Ts
		seiRealTime := seiStruct.RealTs
		fmt.Printf("nowTime,%v,ts,%v,real_ts,%v,"+
			"delay,%v,ts=real_ts,%v\n", nowTime, seiTime, seiRealTime,
			nowTime-seiTime, seiTime == seiRealTime)

		//fmt.Printf("nowTime:%v,seiTime:%v,delay:%v\n", nowTime, seiTime, nowTime-seiTime)
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
