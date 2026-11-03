package accounts

import (
	"context"
	"net"
	"strconv"
	"testing"
)

func TestValidateProxyInput(t *testing.T) {
	valid:=ProxyInput{Name:"Office proxy",Protocol:"SOCKS5",Host:"127.0.0.1",Port:1080,Username:"user",Password:"secret"}
	got,err:=validateProxyInput(valid)
	if err!=nil{t.Fatalf("valid proxy rejected: %v",err)}
	if got.Protocol!="socks5"||got.Name!="Office proxy"{t.Fatalf("input was not normalized: %#v",got)}

	tests:=[]ProxyInput{
		{Name:"",Protocol:"socks5",Host:"localhost",Port:1080},
		{Name:"bad protocol",Protocol:"ftp",Host:"localhost",Port:21},
		{Name:"bad host",Protocol:"http",Host:"http://localhost",Port:80},
		{Name:"bad port",Protocol:"http",Host:"localhost",Port:65536},
	}
	for _,input:=range tests{if _,err:=validateProxyInput(input);err==nil{t.Errorf("expected invalid proxy to be rejected: %#v",input)}}
}

func TestCheckProxyTCPReachability(t *testing.T) {
	listener,err:=net.Listen("tcp","127.0.0.1:0")
	if err!=nil{t.Fatal(err)}
	defer listener.Close()
	port,err:=strconv.Atoi(listener.Addr().(*net.TCPAddr).PortString())
	if err!=nil{t.Fatal(err)}
	if err:=CheckProxy(context.Background(),"127.0.0.1",port);err!=nil{t.Fatalf("reachable endpoint rejected: %v",err)}
}
