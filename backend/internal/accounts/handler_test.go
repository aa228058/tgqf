package accounts

import (
 "net/http/httptest"
 "strings"
 "testing"
)

func TestDecodeRequiresExactlyOneJSONValue(t *testing.T) {
 tests := []struct {
  name string
  body string
  want bool
 }{
  {name:"single object", body:`{"group_name":"friends"}`, want:true},
  {name:"trailing object", body:`{"group_name":"friends"} {}`, want:false},
  {name:"trailing garbage", body:`{"group_name":"friends"} garbage`, want:false},
  {name:"unknown field", body:`{"group_name":"friends","unexpected":true}`, want:false},
  {name:"empty body", body:"", want:false},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
   rec := httptest.NewRecorder()
   var got groupRequest
   if ok := decode(rec, req, &got); ok != tt.want {
    t.Fatalf("decode() = %v, want %v", ok, tt.want)
   }
  })
 }
}

func TestDecodeRejectsBodyOverLimit(t *testing.T) {
 req := httptest.NewRequest("POST", "/", strings.NewReader(`{"group_name":"`+strings.Repeat("x", 1<<20)+`"}`))
 rec := httptest.NewRecorder()
 var got groupRequest
 if decode(rec, req, &got) {
  t.Fatal("decode() accepted a body larger than 1 MiB")
 }
}


func TestSafeCSVCellPreventsFormulaInjection(t *testing.T) {
	for _, value := range []string{"=1+1","+SUM(A1:A2)","-1+2","@SUM(A1)","\tcmd","\rvalue"} {
		if got:=safeCSVCell(value);!strings.HasPrefix(got,"'") { t.Errorf("safeCSVCell(%q)=%q, want apostrophe prefix",value,got) }
	}
	if got:=safeCSVCell("ordinary text");got!="ordinary text" { t.Fatalf("ordinary value changed: %q",got) }
}

func TestNormalizeBulkGroupInput(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	ids, group, err := normalizeBulkGroupInput([]string{" "+id+" "}, "  customers ")
	if err != nil { t.Fatal(err) }
	if len(ids)!=1 || ids[0]!=id || group!="customers" { t.Fatalf("unexpected normalized values: %#v %q",ids,group) }
	if _,_,err:=normalizeBulkGroupInput([]string{id,id},"x");err==nil { t.Fatal("duplicate account IDs accepted") }
	if _,_,err:=normalizeBulkGroupInput([]string{"bad"},"x");err==nil { t.Fatal("invalid account ID accepted") }
	if _,_,err:=normalizeBulkGroupInput(nil,"x");err==nil { t.Fatal("empty account batch accepted") }
	if _,_,err:=normalizeBulkGroupInput([]string{id},strings.Repeat("x",81));err==nil { t.Fatal("oversized group accepted") }
}


func TestNormalizeBulkRuntimeActionInput(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	ids, action, err := normalizeBulkRuntimeActionInput([]string{" " + id + " "}, " RECONNECT ")
	if err != nil { t.Fatal(err) }
	if len(ids) != 1 || ids[0] != id || action != "reconnect" { t.Fatalf("unexpected normalized result: %#v %q", ids, action) }
	if _, _, err := normalizeBulkRuntimeActionInput([]string{id}, "delete"); err == nil { t.Fatal("unsupported action accepted") }
	if _, _, err := normalizeBulkRuntimeActionInput(nil, "disconnect"); err == nil { t.Fatal("empty account selection accepted") }
	if _, _, err := normalizeBulkRuntimeActionInput([]string{id, id}, "disconnect"); err == nil { t.Fatal("duplicate account IDs accepted") }
}

func TestNormalizeBulkPhoneStart(t *testing.T) {
	valid:=bulkPhoneStartRequest{Accounts:[]bulkPhoneStartItem{
		{Phone:"+15551234567",GroupName:" work "},
		{Phone:"447700900123"},
	}}
	got,err:=normalizeBulkPhoneStart(valid)
	if err!=nil { t.Fatal(err) }
	if got.Accounts[0].Phone!="+15551234567" || got.Accounts[0].GroupName!="work" { t.Fatalf("unexpected normalized batch: %#v",got) }
	cases:=[]bulkPhoneStartRequest{
		{},
		{Accounts:make([]bulkPhoneStartItem,21)},
		{Accounts:[]bulkPhoneStartItem{{Phone:"+15551234567"},{Phone:"15551234567"}}},
		{Accounts:[]bulkPhoneStartItem{{Phone:"abcde"}}},
		{Accounts:[]bulkPhoneStartItem{{Phone:"1234"}}},
		{Accounts:[]bulkPhoneStartItem{{Phone:"+1+5551234567"}}},
		{Accounts:[]bulkPhoneStartItem{{Phone:"+15551234567",Proxy:[]byte("[]")}}},
	}
	for i,input:=range cases { if _,err:=normalizeBulkPhoneStart(input);err==nil { t.Errorf("case %d unexpectedly accepted",i) } }
}

func TestNormalizeBulkPhoneAccountIDs(t *testing.T) {
	id:="11111111-1111-4111-8111-111111111111"
	got,err:=normalizeBulkPhoneAccountIDs([]string{" "+id+" "})
	if err!=nil || len(got)!=1 || got[0]!=id { t.Fatalf("unexpected result %#v, %v",got,err) }
	for _,ids:=range [][]string{nil, {id,id}, {"bad"}, make([]string,21)} {
		if _,err:=normalizeBulkPhoneAccountIDs(ids);err==nil { t.Errorf("invalid IDs accepted: %#v",ids) }
	}
}

func TestNormalizeBulkPhoneVerify(t *testing.T) {
	id:="11111111-1111-4111-8111-111111111111"
	got,err:=normalizeBulkPhoneVerify(bulkPhoneVerifyRequest{Accounts:[]bulkPhoneVerifyItem{{AccountID:id,Code:"12345"}}})
	if err!=nil || len(got.Accounts)!=1 { t.Fatalf("valid OTP batch rejected: %#v %v",got,err) }
	_,err=normalizeBulkPhoneVerify(bulkPhoneVerifyRequest{Accounts:[]bulkPhoneVerifyItem{{AccountID:id,Password:"two-factor-secret"}}})
	if err!=nil { t.Fatalf("valid 2FA batch rejected: %v",err) }
	cases:=[]bulkPhoneVerifyRequest{
		{},
		{Accounts:make([]bulkPhoneVerifyItem,21)},
		{Accounts:[]bulkPhoneVerifyItem{{AccountID:id}}},
		{Accounts:[]bulkPhoneVerifyItem{{AccountID:id,Code:"12345",Password:"secret"}}},
		{Accounts:[]bulkPhoneVerifyItem{{AccountID:"bad",Code:"12345"}}},
		{Accounts:[]bulkPhoneVerifyItem{{AccountID:id,Code:"12345"},{AccountID:id,Password:"secret"}}},
	}
	for i,input:=range cases { if _,err:=normalizeBulkPhoneVerify(input);err==nil { t.Errorf("case %d unexpectedly accepted",i) } }
}

func TestNormalizeBulkDeleteAccountIDs(t *testing.T) {
	id:="11111111-1111-4111-8111-111111111111"
	got,err:=normalizeBulkDeleteAccountIDs([]string{" "+id+" "})
	if err!=nil||len(got)!=1||got[0]!=id{t.Fatalf("unexpected normalized IDs %#v, %v",got,err)}
	for _,ids:=range [][]string{nil, {id,id}, {"invalid"}, make([]string,101)} {
		if _,err:=normalizeBulkDeleteAccountIDs(ids);err==nil{t.Errorf("invalid delete IDs accepted: %#v",ids)}
	}
}
