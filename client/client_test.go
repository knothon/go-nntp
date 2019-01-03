package nntpclient

import (
	"bytes"
	"fmt"
	"testing"
	//	"encoding/hex"
	"errors"
	"strings"
)

type stubResponse struct {
	ResponseCode int
	ResponseMsg  string
	HasPayload   bool
	Payload      []string
}
type stubReaderWriter struct {
	receivedRequests []string
	responses        map[string]*stubResponse
	buffer           bytes.Buffer
}

func NewStub(responseCode int, banner string) *stubReaderWriter {
	res := &stubReaderWriter{responses: make(map[string]*stubResponse)}
	res.buffer.WriteString(fmt.Sprintf("%v %v\r\n", responseCode, banner))
	return res
}

func (s *stubReaderWriter) PrepareDotPayloadResponseArray(command string, responseCode int, responseMsg string, payload []string) {
	response := &stubResponse{ResponseCode: responseCode, ResponseMsg: responseMsg, HasPayload: true, Payload: payload}
	s.responses[command] = response
}

func (s *stubReaderWriter) PrepareDotPayloadResponse(command string, responseCode int, responseMsg string, payload ...string) {
	response := &stubResponse{ResponseCode: responseCode, ResponseMsg: responseMsg, HasPayload: true, Payload: payload}
	s.responses[command] = response
}
func (s *stubReaderWriter) PrepareResponse(command string, responseCode int, responseMsg string) {
	response := &stubResponse{ResponseCode: responseCode, ResponseMsg: responseMsg, HasPayload: false}
	s.responses[command] = response
}

func (s *stubReaderWriter) Close() error {
	return nil
}

func (s *stubReaderWriter) Read(p []byte) (n int, err error) {
	return s.buffer.Read(p)
}

func (s *stubReaderWriter) Write(p []byte) (n int, err error) {
	n, err = s.buffer.Write(p)
	//	fmt.Println(hex.EncodeToString(p))
	if err != nil {
		return
	}

	l := len(p)
	if p[l-2] == 0x0d && p[l-1] == 0x0a {
		line := strings.TrimSpace(s.buffer.String())
		s.buffer.Reset()
		cmd := strings.Split(line, " ")[0]
		//		fmt.Println(cmd)
		resp, exists := s.responses[cmd]

		s.receivedRequests = append(s.receivedRequests, cmd)

		if !exists {
			return 0, errors.New("Unknown command")
		}

		s.buffer.WriteString(fmt.Sprintf("%v %v\r\n", resp.ResponseCode, resp.ResponseMsg))
		if resp.HasPayload {
			for _, line := range resp.Payload {
				s.buffer.WriteString(line)
				s.buffer.WriteString("\r\n")
			}
			s.buffer.WriteString(".\r\n")
		}
	}

	return
}

func HasReceivedRequest(s *stubReaderWriter, command string) bool {
	return false
}

func TestCapabilities(t *testing.T) {

	stub := NewStub(200, "Stub")
	stub.PrepareDotPayloadResponse("CAPABILITIES", 101, "Capability list:",
		"COMPRESS DEFLATE SHRINK")
	cli, err := NewConn(stub)
	if err != nil {
		t.Fatal(err)
	}

	cap, err := cli.Capabilities()
	if err != nil {
		t.Fatal(err)
		return
	}
	if len(cap) != 1 {
		t.Fatal("Invalid response, should have length 1")
		return
	}
	if cap[0] != "COMPRESS DEFLATE SHRINK" {

		t.Fatal("Invalid response")
		return
	}
}

func BenchmarkXover(b *testing.B) {
	stub := NewStub(200, "Stub")

	var payload []string
	for i := 0; i < b.N; i++ {
		line := fmt.Sprintf("%v\t[Orphan] Hoshi Neko Full House [1/6] - \"[Orphan] Hoshi Neko Full House - 04 [727A998C].mkv\" yEnc (111/375) 268407965	Anime Tosho <usenet.bot@animetosho.org>	Tue, 28 Nov 2017 20:09:05 GMT\t<XdJjUkOaTsTlNfFfBjWdOfWz-1511899745978@nyuu>		741002	5695	Xref: news.usenetserver.com alt.binaries.multimedia.anime.highspeed:382401874", i)
		payload = append(payload, line)
	}

	stub.PrepareDotPayloadResponse("CAPABILITIES", 101, "Capability list:", "XOVER")
	stub.PrepareDotPayloadResponse("LIST", 215, "List Format:", "Subject:",
		"From:",
		"Date:", "Message-ID:",
		"References:",
		"Bytes:",
		"Lines:",
		"Xref:full")
	stub.PrepareDotPayloadResponseArray("XOVER", 224, "Overview:", payload)
	cli, err := NewConn(stub)
	if err != nil {
		b.Error(err)
		return
	}

	_, err = cli.XOver(0, 100)
	if err != nil {
		b.Error(err)
		return
	}

}

func TestXzver(t *testing.T) {
	stub := NewStub(200, "Stub")
	stub.PrepareDotPayloadResponse("CAPABILITIES", 101, "Capability list:",
		"XZVER")

}

func TestParseDate(t *testing.T) {
	str := "Thu, 03 Jan 19 18:58:44 UTC"
	_, err := parseDate(str)

	if err != nil {
		t.Error(err)
		return
	}

	str = "Thu, 03 Jan 2019 18:58:44 +0000 (UTC)"
	_, err = parseDate(str)

	if err != nil {
		t.Error(err)
		return
	}

}
