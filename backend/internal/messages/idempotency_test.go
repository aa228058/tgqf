package messages

import "testing"

func TestNormalizeIdempotencyKey(t *testing.T) {
	for _,tc:=range []struct{name,key string; wantErr bool}{
		{name:"empty is optional",key:""},
		{name:"valid key",key:"request-123456"},
		{name:"trim whitespace",key:"  request-123456  "},
		{name:"too short",key:"short",wantErr:true},
		{name:"too long",key:"0123456789"+string(make([]byte,119)),wantErr:true},
		{name:"newline",key:"request-123\n456",wantErr:true},
	} {
		t.Run(tc.name,func(t *testing.T){
			got,err:=normalizeIdempotencyKey(tc.key)
			if (err!=nil)!=tc.wantErr {t.Fatalf("error=%v wantErr=%v",err,tc.wantErr)}
			if err==nil && tc.name=="trim whitespace" && got!="request-123456" {t.Fatalf("got %q",got)}
		})
	}
}

func TestReplyRequestHash(t *testing.T) {
	first:=replyRequestHash("message-a","hello")
	if first!=replyRequestHash("message-a","hello") {t.Fatal("same request must have stable fingerprint")}
	if first==replyRequestHash("message-a","different") {t.Fatal("different text must have different fingerprint")}
	if first==replyRequestHash("message-b","hello") {t.Fatal("different message must have different fingerprint")}
}
