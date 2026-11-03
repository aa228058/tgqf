package accounts

import (
	"context"
	"net"
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
	if _,err:=validateProxyInput(ProxyInput{Name:"proxy",Protocol:"socks5",Host:"localhost",Port:1080,Password:"secret",ClearPassword:true});err==nil{t.Fatal("password and clear_password must be mutually exclusive")}
}

func TestCheckProxyTCPReachability(t *testing.T) {
	listener,err:=net.Listen("tcp","127.0.0.1:0")
	if err!=nil{t.Fatal(err)}
	defer listener.Close()
	port:=listener.Addr().(*net.TCPAddr).Port
	if err:=CheckProxy(context.Background(),"127.0.0.1",port);err!=nil{t.Fatalf("reachable endpoint rejected: %v",err)}
}

func TestValidProxyHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"proxy.example.com", true}, {"proxy-1.example.com", true},
		{"proxy.example.com.", true}, {"192.0.2.10", true}, {"2001:db8::1", true},
		{"", false}, {"proxy:1080", false}, {"https://proxy.example.com", false},
		{"bad host", false}, {"-invalid.example", false}, {"invalid-.example", false},
		{"a..example", false}, {"proxy_1.example", false}, {"2001:db8::zz", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := validProxyHost(tt.host); got != tt.want {
				t.Fatalf("validProxyHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestValidateProxyInputTrimsFields(t *testing.T) {
	got, err := validateProxyInput(ProxyInput{Name:" office ", Protocol:" SOCKS5 ", Host:" proxy.example.com ", Port:1080, Username:" user "})
	if err != nil { t.Fatalf("validateProxyInput() error = %v", err) }
	if got.Name != "office" || got.Protocol != "socks5" || got.Host != "proxy.example.com" || got.Username != "user" {
		t.Fatalf("unexpected normalized input: %+v", got)
	}
}
