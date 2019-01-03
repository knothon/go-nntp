// Package nntpclient provides an NNTP Client.
package nntpclient

import (
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net/textproto"
	"strconv"
	"strings"

	"fmt"
	"time"

	"github.com/araddon/dateparse"
	"github.com/chrisfarms/yenc"
	"github.com/knothon/go-nntp"
)

type OverHeader byte

const (
	OverHeaderSubject    = OverHeader('s')
	OverHeaderFrm        = OverHeader('f')
	OverHeaderXRefFull   = OverHeader('x')
	OverHeaderDate       = OverHeader('d')
	OverHeaderMsgId      = OverHeader('m')
	OverHeaderReferences = OverHeader('r')
	OverHeaderBytes      = OverHeader('b')
	OverHeaderLines      = OverHeader('l')
)

// Client is an NNTP client.
type Client struct {
	conn               *textproto.Conn
	overViewFormat     []OverHeader
	capabilities       []string
	loadedCapabilities bool
	Banner             string
	compress           bool
}

// New connects a client to an NNTP server.
func New(net, addr string) (*Client, error) {
	conn, err := textproto.Dial(net, addr)
	if err != nil {
		return nil, err
	}

	return connect(conn)
}

// New connects a client to an NNTP server using tls
func NewSsl(net string, add string, tlsConfig *tls.Config) (*Client, error) {
	conn, err := tls.Dial(net, add, tlsConfig)
	if err != nil {
		return nil, err
	}
	return NewConn(conn)
}

// NewConn wraps an existing connection, for example one opened with tls.Dial
func NewConn(conn io.ReadWriteCloser) (*Client, error) {
	return connect(textproto.NewConn(conn))
}

func connect(conn *textproto.Conn) (*Client, error) {
	_, msg, err := conn.ReadCodeLine(200)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		Banner: msg,
	}, nil
}

func (c *Client) Capabilities() ([]string, error) {
	if !c.loadedCapabilities {
		_, _, err := c.Command("CAPABILITIES", 101)
		if err != nil {
			return nil, err
		}
		lines, err := c.conn.ReadDotLines()
		if err != nil {
			return nil, err
		}
		c.capabilities = lines
	}

	return c.capabilities, nil
}

// Close this client.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Authenticate against an NNTP server using authinfo user/pass
func (c *Client) Authenticate(user, pass string) (msg string, err error) {
	err = c.conn.PrintfLine("authinfo user %s", user)
	if err != nil {
		return
	}
	_, _, err = c.conn.ReadCodeLine(381)
	if err != nil {
		return
	}

	err = c.conn.PrintfLine("authinfo pass %s", pass)
	if err != nil {
		return
	}
	_, msg, err = c.conn.ReadCodeLine(281)
	return
}

func parsePosting(p string) nntp.PostingStatus {
	switch p {
	case "y":
		return nntp.PostingPermitted
	case "m":
		return nntp.PostingModerated
	}
	return nntp.PostingNotPermitted
}

// List groups
func (c *Client) List(sub string) (rv []nntp.Group, err error) {
	_, _, err = c.Command("LIST "+sub, 215)
	if err != nil {
		return
	}
	var groupLines []string
	groupLines, err = c.conn.ReadDotLines()
	if err != nil {
		return
	}
	rv = make([]nntp.Group, 0, len(groupLines))
	for _, l := range groupLines {
		parts := strings.Split(l, " ")
		high, errh := strconv.ParseInt(parts[1], 10, 64)
		low, errl := strconv.ParseInt(parts[2], 10, 64)
		if errh == nil && errl == nil {
			rv = append(rv, nntp.Group{
				Name:    parts[0],
				High:    high,
				Low:     low,
				Posting: parsePosting(parts[3]),
			})
		}
	}
	return
}

// Group selects a group.
func (c *Client) Group(name string) (rv nntp.Group, err error) {
	var msg string
	_, msg, err = c.Command("GROUP "+name, 211)
	if err != nil {
		return
	}
	// count first last name
	parts := strings.Split(msg, " ")
	if len(parts) != 4 {
		err = errors.New("Don't know how to parse result: " + msg)
	}
	rv.Count, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	rv.Low, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	rv.High, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return
	}
	rv.Name = parts[3]

	return
}

// Article grabs an article
func (c *Client) Article(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("ARTICLE %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(220)
}

// Head gets the headers for an article
func (c *Client) Head(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("HEAD %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(221)
}

// Body gets the body of an article
func (c *Client) Body(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("BODY %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(222)
}

func (c *Client) overviewFmt() (res []OverHeader, err error) {
	_, _, err = c.Command("LIST OVERVIEW.FMT", 215)
	if err != nil {
		return
	}
	lines, err := c.conn.ReadDotLines()
	if err != nil {
		return
	}
	res = make([]OverHeader, 0)
	for _, line := range lines {
		switch line {
		case "Subject:":
			res = append(res, OverHeaderSubject)
			break
		case "From:":
			res = append(res, OverHeaderFrm)
			break
		case "Date:":
			res = append(res, OverHeaderDate)
			break
		case "Message-ID:":
			res = append(res, OverHeaderMsgId)
			break
		case "References:":
			res = append(res, OverHeaderReferences)
			break
		case ":bytes":
		case "Bytes":
			res = append(res, OverHeaderBytes)
			break
		case ":lines":
		case "Lines":
			res = append(res, OverHeaderLines)
			break
		case "Xref:full":
			res = append(res, OverHeaderXRefFull)
			break
		}
	}
	err = nil
	return
}

const (
	SHORT_RFC1123  = "Mon, 02 Jan 06 15:04:05 MST"
	SHORT_RFC1123Z = "Mon, 02 Jan 06 15:04:05 -0700" // RFC1123 with numeric zone
)

func parseDate(str string) (time.Time, error) {
	return dateparse.ParseAny(str)
}

type setter = func(*nntp.ArticleOverview, string) error

var infoSetters = map[OverHeader]setter{
	OverHeaderSubject: func(overview *nntp.ArticleOverview, s string) error {
		overview.Subject = s
		return nil
	},
	OverHeaderFrm: func(overview *nntp.ArticleOverview, s string) error {
		overview.From = s
		return nil
	},
	OverHeaderDate: func(overview *nntp.ArticleOverview, s string) error {
		date, err := parseDate(s)
		if err != nil {
			return err
		}
		overview.Date = date
		return nil
	},
	OverHeaderMsgId: func(overview *nntp.ArticleOverview, s string) error {
		overview.MessageId = s
		return nil
	},
	OverHeaderReferences: func(overview *nntp.ArticleOverview, s string) error {
		overview.References = s
		return nil
	},
	OverHeaderLines: func(overview *nntp.ArticleOverview, s string) error {
		lines, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return err
		}
		overview.Lines = uint32(lines)
		return nil
	},
	OverHeaderXRefFull: func(overview *nntp.ArticleOverview, s string) error {
		overview.XRef = s
		return nil
	},
	OverHeaderBytes: func(overview *nntp.ArticleOverview, s string) error {
		bytes, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return err
		}
		overview.Bytes = uint32(bytes)
		return nil
	},
}

func parseArticleOverview(line string, format []OverHeader) (*nntp.ArticleOverview, error) {
	items := strings.Split(line, "\t")
	res := &nntp.ArticleOverview{}
	id, err := strconv.ParseUint(items[0], 10, 64)
	if err != nil {
		return nil, err
	}
	res.Id = id
	for i := 1; i < len(items) && i-1 < len(format); i++ {
		setter, ok := infoSetters[format[i-1]]
		if ok {
			err := setter(res, items[i])
			if err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

func (c *Client) Over(start int64, end int64) ([]*nntp.ArticleOverview, error) {

	if len(c.overViewFormat) == 0 {
		fmt, err := c.overviewFmt()
		if err != nil {
			return nil, err
		}
		c.overViewFormat = fmt
	}
	cmd := fmt.Sprintf("OVER %v-%v", start, end)
	_, _, err := c.Command(cmd, 224)
	if err != nil {
		return nil, err
	}

	var v []*nntp.ArticleOverview
	for {
		var line string
		line, err = c.conn.ReadLine()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			break
		}

		// Dot by itself marks end; otherwise cut one dot.
		if len(line) > 0 && line[0] == '.' {
			if len(line) == 1 {
				break
			}
			line = line[1:]
		}
		art, err := parseArticleOverview(line, c.overViewFormat)
		if err != nil {
			return nil, err
		}

		v = append(v, art)
	}
	return v, nil
}

func (c *Client) EnableCompression() error {
	_, _, err := c.Command("XFEATURE COMPRESS GZIP", 290)

	if err != nil {
		return err
	}

	c.compress = true
	return nil
}

func (c *Client) readDotLines(f func(line string) error) error {
	if c.compress {
		reader, err := getCompressedReader(c.conn)

		if err != nil {
			return err
		}
		for {
			line, err := reader.ReadString(byte(0x0A))

			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}

			line = strings.TrimSpace(string(line))
			err = f(line)
			if err != nil {
				return err
			}
		}
	} else {
		for {
			var line string
			line, err := c.conn.ReadLine()
			if err != nil {
				if err == io.EOF {
					return io.ErrUnexpectedEOF
				}
				return err
			}

			// Dot by itself marks end; otherwise cut one dot.
			if len(line) > 0 && line[0] == '.' {
				if len(line) == 1 {
					break
				}
				line = line[1:]
			}

			err = f(line)

			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) XOver(start int64, end int64) ([]*nntp.ArticleOverview, error) {

	if len(c.overViewFormat) == 0 {
		fmt, err := c.overviewFmt()
		if err != nil {
			return nil, err
		}
		c.overViewFormat = fmt
	}
	cmd := fmt.Sprintf("XOVER %v-%v", start, end)
	_, _, err := c.Command(cmd, 224)
	if err != nil {
		return nil, err
	}

	var v []*nntp.ArticleOverview

	err = c.readDotLines(func(line string) error {
		art, err := parseArticleOverview(line, c.overViewFormat)
		if err != nil {
			return err
		}

		v = append(v, art)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return v, nil
}

func (c *Client) Xzver(start int64, end int64) ([]*nntp.ArticleOverview, error) {
	if len(c.overViewFormat) == 0 {
		fmt, err := c.overviewFmt()
		if err != nil {
			return nil, err
		}
		c.overViewFormat = fmt
	}
	cmd := fmt.Sprintf("XZVER %v-%v", start, end)
	_, _, err := c.Command(cmd, 224)
	if err != nil {
		return nil, err
	}

	part, err := yenc.Decode(c.conn.R)
	if err != nil {
		return nil, err
	}

	fmt.Println(hex.Dump(part.Body))

	return nil, err
}

func (c *Client) articleish(expected int) (int64, string, io.Reader, error) {
	_, msg, err := c.conn.ReadCodeLine(expected)
	if err != nil {
		return 0, "", nil, err
	}
	parts := strings.SplitN(msg, " ", 2)
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", nil, err
	}
	return n, parts[1], c.conn.DotReader(), nil
}

// Post a new article
//
// The reader should contain the entire article, headers and body in
// RFC822ish format.
func (c *Client) Post(r io.Reader) error {
	err := c.conn.PrintfLine("POST")
	if err != nil {
		return err
	}
	_, _, err = c.conn.ReadCodeLine(340)
	if err != nil {
		return err
	}
	w := c.conn.DotWriter()
	_, err = io.Copy(w, r)
	if err != nil {
		// This seems really bad
		return err
	}
	w.Close()
	_, _, err = c.conn.ReadCodeLine(240)
	return err
}

// Command sends a low-level command and get a response.
//
// This will return an error if the code doesn't match the expectCode
// prefix.  For example, if you specify "200", the response code MUST
// be 200 or you'll get an error.  If you specify "2", any code from
// 200 (inclusive) to 300 (exclusive) will be success.  An expectCode
// of -1 disables this behavior.
func (c *Client) Command(cmd string, expectCode int) (int, string, error) {
	err := c.conn.PrintfLine(cmd)
	if err != nil {
		return 0, "", err
	}
	return c.conn.ReadCodeLine(expectCode)
}
