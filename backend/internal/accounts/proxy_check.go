package accounts

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// checkProxyTunnel verifies proxy protocol negotiation (including credentials)
// and that the proxy can establish a TCP tunnel to a Telegram data-center
// endpoint. It does not claim that an account has authenticated with Telegram.
func checkProxyTunnel(ctx context.Context, protocol, host string, port int, username, password, target string) error {
	if strings.TrimSpace(host) == "" || port < 1 || port > 65535 || target == "" {
		return errors.New("invalid proxy endpoint")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("proxy TCP connection failed: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var rw io.ReadWriter = conn
	if protocol == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("HTTPS proxy TLS handshake failed: %w", err)
		}
		defer tlsConn.Close()
		rw = tlsConn
	}

	switch protocol {
	case "socks5":
		if err := negotiateSOCKS5(rw, username, password, target); err != nil { return err }
	case "socks4":
		if err := negotiateSOCKS4(rw, username, target); err != nil { return err }
	case "http", "https":
		if err := negotiateHTTPConnect(rw, username, password, target); err != nil { return err }
	default:
		return errors.New("unsupported proxy protocol")
	}
	return nil
}

func negotiateSOCKS5(rw io.ReadWriter, username, password, target string) error {
	methods := []byte{0}
	if username != "" { methods = append(methods, 2) }
	hello := append([]byte{5, byte(len(methods))}, methods...)
	if _, err := rw.Write(hello); err != nil { return fmt.Errorf("SOCKS5 method negotiation failed: %w", err) }
	reply := make([]byte, 2)
	if _, err := io.ReadFull(rw, reply); err != nil { return fmt.Errorf("SOCKS5 method response failed: %w", err) }
	if reply[0] != 5 || reply[1] == 0xff { return errors.New("SOCKS5 proxy rejected authentication methods") }
	switch reply[1] {
	case 0:
	case 2:
		if len(username) > 255 || len(password) > 255 { return errors.New("SOCKS5 credentials exceed protocol limits") }
		auth := append([]byte{1, byte(len(username))}, []byte(username)...)
		auth = append(auth, byte(len(password)))
		auth = append(auth, []byte(password)...)
		if _, err := rw.Write(auth); err != nil { return fmt.Errorf("SOCKS5 authentication failed: %w", err) }
		authReply := make([]byte, 2)
		if _, err := io.ReadFull(rw, authReply); err != nil { return fmt.Errorf("SOCKS5 authentication response failed: %w", err) }
		if authReply[1] != 0 { return errors.New("SOCKS5 proxy authentication rejected") }
	default:
		return errors.New("SOCKS5 proxy selected an unsupported authentication method")
	}
	host, portText, err := net.SplitHostPort(target)
	if err != nil { return fmt.Errorf("invalid tunnel target: %w", err) }
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 { return errors.New("invalid tunnel target port") }
	request := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil { request = append(request, 1); request = append(request, v4...)
		} else { request = append(request, 4); request = append(request, ip.To16()...) }
	} else {
		if len(host) > 255 { return errors.New("tunnel target hostname is too long") }
		request = append(request, 3, byte(len(host)))
		request = append(request, []byte(host)...)
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))
	request = append(request, portBytes[:]...)
	if _, err := rw.Write(request); err != nil { return fmt.Errorf("SOCKS5 CONNECT failed: %w", err) }
	header := make([]byte, 4)
	if _, err := io.ReadFull(rw, header); err != nil { return fmt.Errorf("SOCKS5 CONNECT response failed: %w", err) }
	if header[0] != 5 || header[1] != 0 { return fmt.Errorf("SOCKS5 CONNECT rejected (code %d)", header[1]) }
	var addressLength int
	switch header[3] { case 1: addressLength = 4; case 4: addressLength = 16; case 3: b:=make([]byte,1); if _,err:=io.ReadFull(rw,b);err!=nil{return err};addressLength=int(b[0]); default: return errors.New("SOCKS5 proxy returned an invalid address type") }
	if _, err := io.CopyN(io.Discard, rw, int64(addressLength+2)); err != nil { return fmt.Errorf("SOCKS5 CONNECT response address failed: %w", err) }
	return nil
}

func negotiateSOCKS4(rw io.ReadWriter, username, target string) error {
	host, portText, err := net.SplitHostPort(target)
	if err != nil { return fmt.Errorf("invalid tunnel target: %w", err) }
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 { return errors.New("invalid tunnel target port") }
	request := []byte{4, 1, byte(port >> 8), byte(port), 0, 0, 0, 1}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil { copy(request[4:8], v4) } else { return errors.New("SOCKS4 does not support IPv6 tunnel targets") }
	}
	request = append(request, []byte(username)...)
	request = append(request, 0)
	if net.ParseIP(host) == nil {
		if len(host) > 255 { return errors.New("tunnel target hostname is too long") }
		request = append(request, []byte(host)...)
		request = append(request, 0)
	}
	if _, err := rw.Write(request); err != nil { return fmt.Errorf("SOCKS4 CONNECT failed: %w", err) }
	reply := make([]byte, 8)
	if _, err := io.ReadFull(rw, reply); err != nil { return fmt.Errorf("SOCKS4 CONNECT response failed: %w", err) }
	if reply[1] != 90 { return fmt.Errorf("SOCKS4 CONNECT rejected (code %d)", reply[1]) }
	return nil
}

func negotiateHTTPConnect(rw io.ReadWriter, username, password, target string) error {
	req, err := http.NewRequest(http.MethodConnect, "http://"+target, nil)
	if err != nil { return err }
	req.Host = target
	if username != "" {
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(rw); err != nil { return fmt.Errorf("HTTP CONNECT failed: %w", err) }
	reader := bufio.NewReader(rw)
	resp, err := http.ReadResponse(reader, req)
	if err != nil { return fmt.Errorf("HTTP CONNECT response failed: %w", err) }
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP CONNECT rejected: %s", textproto.TrimString(resp.Status))
	}
	// CONNECT responses have no body; do not consume bytes from the tunnel.
	return nil
}
