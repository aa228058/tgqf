package accounts

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestNegotiateSOCKS5WithUsernamePassword(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		hello := make([]byte, 4)
		if _, err := io.ReadFull(server, hello[:2]); err != nil { done <- err; return }
		methods := make([]byte, int(hello[1]))
		if _, err := io.ReadFull(server, methods); err != nil { done <- err; return }
		if hello[0] != 5 || len(methods) != 2 || methods[1] != 2 { done <- fmt.Errorf("unexpected methods: %v %v", hello, methods); return }
		_, _ = server.Write([]byte{5, 2})
		authHead := make([]byte, 2)
		if _, err := io.ReadFull(server, authHead); err != nil { done <- err; return }
		user := make([]byte, int(authHead[1]))
		if _, err := io.ReadFull(server, user); err != nil { done <- err; return }
		passLen := []byte{0}
		if _, err := io.ReadFull(server, passLen); err != nil { done <- err; return }
		pass := make([]byte, int(passLen[0]))
		if _, err := io.ReadFull(server, pass); err != nil { done <- err; return }
		if string(user) != "alice" || string(pass) != "secret" { done <- fmt.Errorf("wrong credentials %q/%q", user, pass); return }
		_, _ = server.Write([]byte{1, 0})
		requestHead := make([]byte, 5)
		if _, err := io.ReadFull(server, requestHead); err != nil { done <- err; return }
		if requestHead[0] != 5 || requestHead[1] != 1 || requestHead[3] != 3 { done <- fmt.Errorf("invalid connect header: %v", requestHead); return }
		name := make([]byte, int(requestHead[4])+2)
		if _, err := io.ReadFull(server, name); err != nil { done <- err; return }
		if string(name[:len(name)-2]) != "telegram.org" { done <- fmt.Errorf("wrong target: %q", name); return }
		_, _ = server.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0x01, 0xbb})
		done <- nil
	}()
	if err := negotiateSOCKS5(client, "alice", "secret", "telegram.org:443"); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestNegotiateSOCKS5RejectsAuthentication(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { head:=make([]byte,2);_,_=io.ReadFull(server,head);methods:=make([]byte,int(head[1]));_,_=io.ReadFull(server,methods);_,_=server.Write([]byte{5,2});auth:=make([]byte,2);_,_=io.ReadFull(server,auth);u:=make([]byte,int(auth[1]));_,_=io.ReadFull(server,u);n:=[]byte{0};_,_=io.ReadFull(server,n);p:=make([]byte,int(n[0]));_,_=io.ReadFull(server,p);_,_=server.Write([]byte{1,1}) }()
	if err := negotiateSOCKS5(client, "alice", "bad", "telegram.org:443"); err == nil { t.Fatal("rejected credentials unexpectedly succeeded") }
}

func TestNegotiateHTTPConnectSendsBasicCredentials(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		req, err := http.ReadRequest(bufio.NewReader(server))
		if err != nil { done <- err; return }
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret"))
		if req.Method != http.MethodConnect || req.Host != "telegram.org:443" || req.Header.Get("Proxy-Authorization") != want { done <- fmt.Errorf("unexpected CONNECT request: method=%s host=%s auth=%s", req.Method, req.Host, req.Header.Get("Proxy-Authorization")); return }
		_, err = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done <- err
	}()
	if err := negotiateHTTPConnect(client, "alice", "secret", "telegram.org:443"); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestNegotiateHTTPConnectLeavesTunnelUsable(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		req, err := http.ReadRequest(bufio.NewReader(server))
		if err != nil { done <- err; return }
		if req.Method != http.MethodConnect { done <- fmt.Errorf("unexpected method %s", req.Method); return }
		if _, err := io.WriteString(server, "HTTP/1.1 200 Connection Established\\r\\n\\r\\n"); err != nil { done <- err; return }
		payload := make([]byte, 4)
		if _, err := io.ReadFull(server, payload); err != nil { done <- err; return }
		if string(payload) != "ping" { done <- fmt.Errorf("unexpected tunnel payload %q", payload); return }
		done <- nil
	}()
	if err := negotiateHTTPConnect(client, "", "", "telegram.org:443"); err != nil { t.Fatal(err) }
	if _, err := io.WriteString(client, "ping"); err != nil { t.Fatalf("CONNECT tunnel was closed after success: %v", err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestNegotiateHTTPConnectRejectsProxyFailure(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _,_ = http.ReadRequest(bufio.NewReader(server)); _,_ = io.WriteString(server, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n") }()
	if err := negotiateHTTPConnect(client, "", "", "telegram.org:443"); err == nil { t.Fatal("proxy rejection unexpectedly succeeded") }
}

func TestNegotiateSOCKS4Connect(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		req:=make([]byte,9)
		if _,err:=io.ReadFull(server,req);err!=nil{done<-err;return}
		if req[0]!=4||req[1]!=1||string(req[8:])!="\x00"{done<-fmt.Errorf("invalid SOCKS4 request: %v",req);return}
		_,err:=server.Write([]byte{0,90,0,0,0,0,0,0})
		done<-err
	}()
	if err:=negotiateSOCKS4(client,"","192.0.2.10:443");err!=nil{t.Fatal(err)}
	if err:=<-done;err!=nil{t.Fatal(err)}
}

func TestCheckProxyTunnelRejectsUnknownProtocol(t *testing.T) {
	if err:=checkProxyTunnel(t.Context(),"unknown","127.0.0.1",1,"","","telegram.org:443");err==nil{t.Fatal("unknown protocol accepted")}
}

var _ net.Conn
