package nntpclient

import (
	"bufio"
	"compress/zlib"
	"io"
	"net/textproto"
	"strings"

	"github.com/traherom/memstream"
)

func readCompressed(proto *textproto.Conn, istream *memstream.MemoryStream) error {

	buf := make([]byte, 1024, 1024)
	rb := newRingBuffer(3)
	term := []byte(".\r\n")

	for {
		br, err := proto.R.Read(buf)
		if err != nil {
			return err
		}

		sl := buf[0:br]
		rb.Write(sl)

		if rb.Equals(term) {
			istream.Write(sl[0 : br-3])
			return nil
		}

		istream.Write(sl)
	}
}

func getCompressedReader(proto *textproto.Conn) (*bufio.Reader, error) {
	iostream := memstream.NewCapacity(1024 * 32)

	err := readCompressed(proto, iostream)
	if err != nil {
		return nil, err
	}

	iostream.Seek(0, 0)

	zr, err := zlib.NewReader(iostream)
	if err != nil {
		return nil, err
	}
	return bufio.NewReader(zr), nil
}

func readCompressedLines(proto *textproto.Conn) ([]string, error) {
	reader, err := getCompressedReader(proto)

	if err != nil {
		return nil, err
	}
	res := make([]string, 0, 0)
	for {
		line, err := reader.ReadString(byte(0x0A))

		if err == io.EOF {
			return res, nil
		}
		if err != nil {
			return nil, err
		}

		res = append(res, strings.TrimSpace(string(line)))
	}

	return res, nil
}
