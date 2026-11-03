package accounts

import "testing"

func TestNormalizeAccountListOptions(t *testing.T) {
	got,err:=normalizeAccountListOptions(AccountListOptions{Query:"  Alice  ",GroupName:" work ",Status:" AUTHORIZED ",Limit:0,Offset:-2})
	if err!=nil { t.Fatal(err) }
	if got.Query!="Alice"||got.GroupName!="work"||got.Status!="authorized"||got.Limit!=50||got.Offset!=0 {
		t.Fatalf("normalized options = %+v",got)
	}
	if _,err:=normalizeAccountListOptions(AccountListOptions{Status:"unknown"});err==nil { t.Fatal("unknown status should be rejected") }
	if _,err:=normalizeAccountListOptions(AccountListOptions{Query:string(make([]byte,121))});err==nil { t.Fatal("oversized query should be rejected") }
}
