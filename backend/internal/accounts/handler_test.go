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
