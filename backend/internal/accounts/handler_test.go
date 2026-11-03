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
