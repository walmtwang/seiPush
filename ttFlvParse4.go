package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-flv"
	"net/http"
	"os"
	"strings"
	"time"
)

type E2eDelaySei struct {
	VideoE2EDelay struct {
		CaptureCamera int64 `json:"capture_camera"`
		Encode        int64 `json:"encode"`
		PushEdgeTS    int64 `json:"push_edge_ts"`
		OriginTs      int64 `json:"origin_ts"`
		PullEdgeTS    int64 `json:"pull_edge_ts"`
	} `json:"video_e2e_delay"`
	SEIIndex int `json:"sei_index"`
}

const (
	programName = "FlvParse"
	version     = "0.0.1"
)

var (
	url *string = flag.String("URL", "http://domain/stage/TestDelay20230426.flv", "The rtmp url to connect.")
)

func splitNALU(data []byte, cid codec.CodecID) [][]byte {
	var nalus [][]byte
	var startCode []byte

	if cid == codec.CODECID_VIDEO_H264 {
		startCode = []byte{0x00, 0x00, 0x00, 0x01}
	} else {
		startCode = []byte{0x00, 0x00, 0x00, 0x01}
	}

	pos := 0
	for pos < len(data) {
		// 查找起始码
		nextPos := -1
		for i := pos; i <= len(data)-4; i++ {
			if bytes.Equal(data[i:i+4], startCode) {
				if i > pos && i-pos > 4 {
					nextPos = i
					break
				}
			}
		}

		if nextPos == -1 {
			// 没有找到更多NALU
			if len(data)-pos > 4 {
				nalus = append(nalus, data[pos:])
			}
			break
		}

		if nextPos > pos && nextPos-pos > 4 {
			nalus = append(nalus, data[pos:nextPos])
		}
		pos = nextPos + 4
	}

	return nalus
}

func extractSEIPayload(nalu []byte, cid codec.CodecID) ([][]byte, error) {
	var payloads [][]byte
	pos := 0

	// 跳过NALU头
	if cid == codec.CODECID_VIDEO_H264 {
		pos = 1
	} else if cid == codec.CODECID_VIDEO_H265 {
		pos = 2
	}

	for pos < len(nalu) {
		// 解析payload_type
		payloadType := 0
		for pos < len(nalu) {
			b := nalu[pos]
			pos++
			payloadType += int(b)
			if b != 0xFF {
				break
			}
			if pos >= len(nalu) {
				return payloads, nil
			}
		}

		// 解析payload_size
		payloadSize := 0
		for pos < len(nalu) {
			b := nalu[pos]
			pos++
			payloadSize += int(b)
			if b != 0xFF {
				break
			}
			if pos >= len(nalu) {
				return payloads, nil
			}
		}

		if payloadSize == 0 {
			// payload_size为0表示是结束符
			break
		}

		if pos+payloadSize > len(nalu) {
			break
		}

		payload := nalu[pos : pos+payloadSize]
		pos += payloadSize

		if payloadType == 100 {
			payloads = append(payloads, payload)
		}

		// 跳过rbsp_trailing_bits
		if pos < len(nalu) && nalu[pos] == 0x80 {
			pos++
		}
	}

	return payloads, nil
}

func parseSEIFromNALU(nalu []byte, cid codec.CodecID) *E2eDelaySei {
	if len(nalu) < 2 {
		return nil
	}

	var naluType byte
	if cid == codec.CODECID_VIDEO_H264 {
		naluType = nalu[0] & 0x1F
		if naluType != 6 { // H.264 SEI是type 6
			return nil
		}
	} else if cid == codec.CODECID_VIDEO_H265 {
		naluType = (nalu[0] >> 1) & 0x3F
		if naluType != 39 && naluType != 40 { // H.265 SEI是type 39(Prefix)或40(Suffix)
			return nil
		}
	} else {
		return nil
	}

	// 解析SEI payload
	payload, err := extractSEIPayload(nalu, cid)
	if err != nil {
		return nil
	}

	// 尝试解析为JSON
	for _, data := range payload {
		var seiData E2eDelaySei
		if err := json.Unmarshal(data, &seiData); err == nil {
			// 验证是否是我们要的格式
			if seiData.VideoE2EDelay.CaptureCamera > 0 {
				return &seiData
			}
		}

		// 尝试从字符串中提取JSON
		strData := string(data)
		if idx := strings.Index(strData, "{"); idx != -1 {
			if endIdx := strings.LastIndex(strData, "}"); endIdx > idx {
				jsonStr := strData[idx : endIdx+1]
				if err := json.Unmarshal([]byte(jsonStr), &seiData); err == nil {
					if seiData.VideoE2EDelay.CaptureCamera > 0 {
						return &seiData
					}
				}
			}
		}
	}

	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s version[%s]\r\nUsage: %s [OPTIONS]\r\n", programName, version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	req, err := http.NewRequest("GET", *url, nil)
	if err != nil {
		fmt.Printf("http.NewRequest err:%v\n", err)
		return
	}
	req.Header.Set("User-Agent", "Lavf/1.1")
	resp, err := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	fmt.Printf("http status:%v\n", resp.StatusCode)
	if resp.StatusCode != 200 {
		return
	}

	flvReader := flv.CreateFlvReader()
	flvReader.OnFrame = func(cid codec.CodecID, data []byte, pts, dts uint32) {
		if len(data) < 5 {
			return
		}
		if cid != codec.CODECID_VIDEO_H264 && cid != codec.CODECID_VIDEO_H265 {
			return
		}

		nowTime := time.Now().UnixNano() / 1e6

		// 跳过FLV视频Tag头
		var nalus [][]byte

		nalus = splitNALU(data, cid)

		// 解析每个NALU寻找SEI
		for _, nalu := range nalus {
			if sei := parseSEIFromNALU(nalu, cid); sei != nil {
				v := sei.VideoE2EDelay
				fmt.Printf("captureCamera:%v,pushEdgeTs:%v,pullEdgeTs:%v,nowTime:%v\n", v.CaptureCamera, v.PushEdgeTS, v.PullEdgeTS, nowTime)
				fmt.Printf("pushEdgeTs-captureCamera:%v,pullEdgeTs-pushEdgeTs:%v,nowTime-pullEdgeTs:%v\n", v.PushEdgeTS-v.CaptureCamera, v.PullEdgeTS-v.PushEdgeTS, nowTime-v.PullEdgeTS)
				return
			}
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

func BytesToInt64(buf []byte) int64 {
	return int64(binary.BigEndian.Uint64(buf))
}
