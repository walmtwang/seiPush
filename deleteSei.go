package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"

	flv "github.com/zhangpeihao/goflv"
)

var (
	inputFile  *string = flag.String("inputFile", "./clock_av.flv", "FLV file to input.")
	outputFile *string = flag.String("outputFile", "./clock_av.flv", "FLV file to output.")
)

func deleteSeiNalu(header *flv.TagHeader, data []byte) (*flv.TagHeader, []byte) {

	if header.TagType != flv.VIDEO_TAG {
		return header, data
	}
	codecId := data[0] & 0b00001111
	if codecId != 7 {
		return header, data
	}
	avcPacketType := data[1]
	if avcPacketType != 1 {
		return header, data
	}

	if len(data) <= 9 {
		return header, data
	}
	//fmt.Printf("%x\n", data)
	firstNaluSize := binary.BigEndian.Uint32(data[5:9])
	naluData := data[9 : 9+int(firstNaluSize)]
	if naluData[0] != 0x06 {
		return header, data
	}

	header.DataSize -= 4 + uint32(firstNaluSize)

	newData := bytes.Join([][]byte{data[:5], data[9+int(firstNaluSize):]}, []byte{})

	return header, newData
}

func Int64ToBytes(i int64) []byte {
	var buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i))
	return buf
}

func BytesToInt64(buf []byte) int64 {
	return int64(binary.BigEndian.Uint64(buf))
}

func BytesToInt32(buf []byte) int64 {
	return int64(binary.BigEndian.Uint32(buf))
}

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()

	inputFile, err := flv.OpenFile(*inputFile)
	if err != nil {
		fmt.Println("Open FLV dump file error:", err)
		return
	}
	defer inputFile.Close()

	outputFile, err := flv.CreateFile(*outputFile)
	if err != nil {
		fmt.Println("flv.CreateFile error:", err)
		return
	}
	defer outputFile.Close()

	for !inputFile.IsFinished() {

		header, data, err := inputFile.ReadTag()
		if err != nil {
			fmt.Println("flvFile.ReadTag() error:", err)
			break
		}

		header, data = deleteSeiNalu(header, data)

		if err = outputFile.WriteTag(data, header.TagType, header.Timestamp); err != nil {
			fmt.Println("outputFile.WriteTag error:", err)
			break
		}
	}
	fmt.Println("@@@@@@@@@@@@@@File finished")
}
