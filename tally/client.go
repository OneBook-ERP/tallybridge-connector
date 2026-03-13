package tally

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// invalidXMLCharRef matches XML numeric character references for code points
// that are forbidden in XML 1.0: U+0001–U+0008, U+000B, U+000C, U+000E–U+001F.
// Tally sometimes emits &#4; (EOT) as a prefix on system master names (e.g.
// "&#4; Primary").  Go's encoding/xml rejects these and returns a parse error,
// silently returning 0 records unless we strip them first.
var invalidXMLCharRef = regexp.MustCompile(`&#([0-9]{1,5}|x[0-9a-fA-F]{1,4});`)

// sanitizeTallyXML removes invalid XML 1.0 content from the raw Tally response.
// Tally can emit two kinds of invalid content:
//  1. XML numeric character references for forbidden code points (e.g. &#4;)
//  2. Raw byte values in the forbidden range (0x01–0x08, 0x0B, 0x0C, 0x0E–0x1F)
//     embedded directly in text/attribute values of large responses (ledgers, vouchers)
//
// Both cause Go's encoding/xml to abort the entire parse and return 0 records.
func sanitizeTallyXML(b []byte) []byte {
	// Pass 1: strip &#N; / &#xN; character references for forbidden code points.
	b = invalidXMLCharRef.ReplaceAllFunc(b, func(ref []byte) []byte {
		s := string(ref)
		var cp int
		if s[2] == 'x' || s[2] == 'X' {
			fmt.Sscanf(s[3:len(s)-1], "%x", &cp)
		} else {
			fmt.Sscanf(s[2:len(s)-1], "%d", &cp)
		}
		// Keep: TAB (9), LF (10), CR (13), space and above (≥32)
		if cp == 9 || cp == 10 || cp == 13 || cp >= 32 {
			return ref
		}
		return nil // strip invalid control character
	})
	// Collapse any &#13;&#10; or &#13; that remain inside attribute/text values.
	// These show up as trailing whitespace in ledger names.
	b = bytes.ReplaceAll(b, []byte("&#13;&#10;"), []byte(""))
	b = bytes.ReplaceAll(b, []byte("&#13;"), []byte(""))

	// Pass 2: strip raw bytes in the XML 1.0 forbidden range.
	// Allowed: 0x09 (TAB), 0x0A (LF), 0x0D (CR), 0x20–0xFF.
	// Forbidden: 0x01–0x08, 0x0B, 0x0C, 0x0E–0x1F.
	// This is needed for large responses (ledger list, vouchers) where Tally
	// sometimes embeds raw control bytes rather than &#N; references.
	out := b[:0:len(b)] // reuse the same backing array
	for _, c := range b {
		if c < 0x20 && c != 0x09 && c != 0x0A && c != 0x0D {
			continue // forbidden control byte — drop it
		}
		out = append(out, c)
	}
	b = out

	// Pass 3: replace invalid UTF-8 sequences.
	// Tally ERP 9 (Windows) sometimes emits CP-1252/Latin-1 bytes (e.g. ₹ as 0x80,
	// curly quotes, em-dashes) in ledger names without re-encoding to UTF-8.
	// Go's encoding/xml rejects invalid UTF-8 with "XML syntax error: invalid UTF-8".
	// Strip those bytes so the XML structure remains intact.
	if !utf8.Valid(b) {
		b = []byte(strings.ToValidUTF8(string(b), ""))
	}
	return b
}

const (
	// DefaultTallyAddr is where Tally's XML server listens on the client machine.
	DefaultTallyAddr = "http://localhost:9000"

	// defaultTimeout is for read/master queries. Increased from 30s to 120s
	// because large companies (e.g. Covai Polymer Traders) can take over a
	// minute to process even a single month's worth of vouchers.
	defaultTimeout = 120 * time.Second
)

// Client is the HTTP client for Tally port 9000.
// All communication with Tally goes through this type.
// It is safe to use from multiple goroutines.
type Client struct {
	addr       string
	httpClient *http.Client
}

// ClientOption is a functional option for Client construction.
type ClientOption func(*Client)

// WithAddr overrides the Tally XML server address (default: http://localhost:9000).
func WithAddr(addr string) ClientOption {
	return func(c *Client) { c.addr = addr }
}

// WithTimeout sets a custom HTTP timeout for all requests made by this client.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient creates a Tally XML API client.
// The address is resolved in this priority order:
//  1. WithAddr option
//  2. TALLY_ADDR environment variable  (set to http://192.168.1.10:9000 for live dev testing)
//  3. DefaultTallyAddr (http://localhost:9000)
func NewClient(opts ...ClientOption) *Client {
	addr := DefaultTallyAddr
	if v := os.Getenv("TALLY_ADDR"); v != "" {
		addr = v
	}
	c := &Client{
		addr: addr,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Post sends a raw XML body to Tally and returns the response body.
// The caller is responsible for parsing the returned XML.
//
// On Tally port timeout, the error will wrap context.DeadlineExceeded.
// The sync scheduler must NOT update lastSyncAt on any error from this method.
func (c *Client) Post(ctx context.Context, xmlBody string) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.addr,
		bytes.NewBufferString(xmlBody),
	)
	if err != nil {
		return nil, fmt.Errorf("tally: build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tally: POST %s: %w", c.addr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tally: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tally: unexpected HTTP %d — body: %s", resp.StatusCode, body)
	}

	return sanitizeTallyXML(body), nil
}

// Ping performs a lightweight check to see if Tally is reachable.
// Returns nil if Tally responds (even to an empty/malformed request),
// non-nil if the port is unreachable or times out.
// Use a short context (e.g. 5 s) for heartbeats.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Post(ctx, "<ENVELOPE/>")
	// Tally responds with an XML error even to malformed requests —
	// what matters is whether the TCP connection succeeded.
	// We treat any HTTP response (even an error body) as "reachable".
	if err != nil {
		// Distinguish connection-refused / timeout from application errors.
		return fmt.Errorf("tally: ping: %w", err)
	}
	return nil
}
