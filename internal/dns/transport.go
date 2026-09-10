package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// maxUDPMessageSize is the buffer nett reads a UDP response into. Answers
// nett cares about (A/AAAA/CNAME/MX/NS/TXT/SRV/CAA/PTR/SOA for a handful of
// names) never approach the 64KiB DNS message ceiling; this is generous
// headroom, not a protocol limit nett is relying on being tight.
const maxUDPMessageSize = 4096

// exchangeUDP sends msg to addr over UDP and returns the parsed response.
func exchangeUDP(ctx context.Context, addr string, msg *Message, timeout time.Duration) (*Message, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dns: dial udp %s: %w", addr, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("dns: set deadline: %w", err)
	}

	req, err := msg.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("dns: write udp %s: %w", addr, err)
	}

	buf := make([]byte, maxUDPMessageSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("dns: read udp %s: %w", addr, err)
		}
		resp, err := Unmarshal(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("dns: parse response from %s: %w", addr, err)
		}
		if resp.ID != msg.ID {
			// A stray/stale reply on this socket; keep waiting for ours
			// until the deadline set above fires.
			continue
		}
		return resp, nil
	}
}

// exchangeTCP sends msg to addr over TCP using the 2-byte length prefix
// RFC 1035 §4.2.2 requires for the stream transport.
func exchangeTCP(ctx context.Context, addr string, msg *Message, timeout time.Duration) (*Message, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dns: dial tcp %s: %w", addr, err)
	}
	defer conn.Close()
	return exchangeStream(conn, msg, deadlineFor(ctx, timeout))
}

// exchangeDoT is exchangeTCP over a TLS connection (RFC 7858): the same
// 2-byte length-prefixed wire format, tunneled through TLS instead of plain
// TCP so the query is confidential and authenticated in transit.
func exchangeDoT(ctx context.Context, addr string, msg *Message, timeout time.Duration, tlsConfig *tls.Config) (*Message, error) {
	var d net.Dialer
	dl := deadlineFor(ctx, timeout)
	dctx, cancel := context.WithDeadline(ctx, dl)
	defer cancel()
	conn, err := (&tls.Dialer{NetDialer: &d, Config: tlsConfig}).DialContext(dctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dns: dial dot %s: %w", addr, err)
	}
	defer conn.Close()
	return exchangeStream(conn, msg, dl)
}

func exchangeStream(conn net.Conn, msg *Message, deadline time.Time) (*Message, error) {
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("dns: set deadline: %w", err)
	}
	req, err := msg.Marshal()
	if err != nil {
		return nil, err
	}
	var lenPrefix [2]byte
	binary.BigEndian.PutUint16(lenPrefix[:], uint16(len(req)))
	if _, err := conn.Write(append(lenPrefix[:], req...)); err != nil {
		return nil, fmt.Errorf("dns: write stream: %w", err)
	}

	if _, err := io.ReadFull(conn, lenPrefix[:]); err != nil {
		return nil, fmt.Errorf("dns: read length prefix: %w", err)
	}
	respLen := binary.BigEndian.Uint16(lenPrefix[:])
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, fmt.Errorf("dns: read stream body: %w", err)
	}
	return Unmarshal(respBuf)
}

func deadlineFor(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	return deadline
}

// dohMediaType is the wire-format media type RFC 8484 defines for DoH.
const dohMediaType = "application/dns-message"

// exchangeDoH sends msg as a DoH (RFC 8484) POST to serverURL (e.g.
// "https://1.1.1.1/dns-query") and returns the parsed response.
func exchangeDoH(ctx context.Context, client *http.Client, serverURL string, msg *Message) (*Message, error) {
	req, err := msg.Marshal()
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(req))
	if err != nil {
		return nil, fmt.Errorf("dns: build doh request: %w", err)
	}
	httpReq.Header.Set("Content-Type", dohMediaType)
	httpReq.Header.Set("Accept", dohMediaType)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dns: doh request to %s: %w", serverURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dns: doh server %s returned HTTP %d", serverURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUDPMessageSize*4))
	if err != nil {
		return nil, fmt.Errorf("dns: read doh response: %w", err)
	}
	return Unmarshal(body)
}
