package messages

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeMessageJSONRequiresExactlyOneValue(t *testing.T) {
	tests := []struct {
		name string
		body string
		wantErr bool
	}{
		{name:"single object",body:`{"enabled":true}`},
		{name:"trailing object",body:`{"enabled":true} {}`,wantErr:true},
		{name:"trailing garbage",body:`{"enabled":true} garbage`,wantErr:true},
		{name:"unknown field",body:`{"enabled":true,"unexpected":true}`,wantErr:true},
		{name:"empty body",body:"",wantErr:true},
	}
	for _,tt:=range tests {
		t.Run(tt.name,func(t *testing.T){
			req:=httptest.NewRequest("PUT","/",strings.NewReader(tt.body))
			rec:=httptest.NewRecorder()
			var got struct{Enabled bool `json:"enabled"`}
			err:=decodeMessageJSON(rec,req,1024,&got)
			if (err!=nil)!=tt.wantErr { t.Fatalf("decodeMessageJSON() error=%v, wantErr=%v",err,tt.wantErr) }
		})
	}
}

func TestDecodeMessageJSONRejectsOversizedBody(t *testing.T) {
	req:=httptest.NewRequest("PUT","/",strings.NewReader(`{"value":"`+strings.Repeat("x",2048)+`"}`))
	rec:=httptest.NewRecorder()
	var got map[string]any
	if err:=decodeMessageJSON(rec,req,1024,&got);err==nil { t.Fatal("oversized body was accepted") }
}
