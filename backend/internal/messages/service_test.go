package messages

import ("testing"; "strings")

func TestShouldCollect(t *testing.T) {
	tests:=[]struct{name,chatType string;outgoing,replyToOwn,want bool}{
		{"incoming private","private",false,false,true},
		{"outgoing private","private",true,false,false},
		{"incoming group without reply","group",false,false,false},
		{"incoming group reply to own","group",false,true,true},
		{"outgoing group reply","group",true,true,false},
		{"broadcast channel","channel",false,true,false},
		{"bot peer","bot",false,false,false},
		{"unknown peer","unknown",false,true,false},
	}
	for _,tt:=range tests{t.Run(tt.name,func(t *testing.T){if got:=ShouldCollect(tt.chatType,tt.outgoing,tt.replyToOwn);got!=tt.want{t.Fatalf("ShouldCollect()=%v want %v",got,tt.want)}})}
}

func TestValidUUID(t *testing.T) {
	if !validUUID("8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234"){t.Fatal("valid UUID rejected")}
	for _,value:=range []string{"","not-a-uuid","8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234x","8f14e45f_ea3e_4e3d_9c2d_5a9f6b8a1234"}{
		if validUUID(value){t.Errorf("invalid UUID accepted: %q",value)}
	}
}


func TestCollectionSettingsAllows(t *testing.T) {
 base := DefaultCollectionSettings()
 incoming := Incoming{AccountID:"8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234", ChatID:-100123, Text:"Please review the invoice", ChatType:"group", ReplyToOwnMessage:true}
 tests := []struct{name string;settings CollectionSettings;incoming Incoming;want bool}{
  {"defaults accept eligible text",base,incoming,true},
  {"disabled rejects",func()CollectionSettings{s:=base;s.Enabled=false;return s}(),incoming,false},
  {"account allow list rejects",func()CollectionSettings{s:=base;s.IncludeAllAccounts=false;s.AccountIDs=[]string{"11111111-1111-4111-8111-111111111111"};return s}(),incoming,false},
  {"account allow list accepts",func()CollectionSettings{s:=base;s.IncludeAllAccounts=false;s.AccountIDs=[]string{incoming.AccountID};return s}(),incoming,true},
  {"chat allow list rejects",func()CollectionSettings{s:=base;s.ChatIDs=[]int64{-100456};return s}(),incoming,false},
  {"chat allow list accepts",func()CollectionSettings{s:=base;s.ChatIDs=[]int64{incoming.ChatID};return s}(),incoming,true},
  {"text disabled rejects",func()CollectionSettings{s:=base;s.IncludeText=false;return s}(),incoming,false},
  {"media enabled accepts",func()CollectionSettings{s:=base;s.IncludeText=false;return s}(),func()Incoming{i:=incoming;i.Text="";i.MediaType="photo";return i}(),true},
  {"media disabled rejects",func()CollectionSettings{s:=base;s.IncludeText=false;s.IncludeMedia=false;return s}(),func()Incoming{i:=incoming;i.Text="";i.MediaType="photo";return i}(),false},
  {"keyword matching case insensitive",func()CollectionSettings{s:=base;s.Keywords=[]string{"INVOICE"};return s}(),incoming,true},
  {"keyword miss rejects",func()CollectionSettings{s:=base;s.Keywords=[]string{"urgent"};return s}(),incoming,false},
  {"keyword rejects media without text",func()CollectionSettings{s:=base;s.Keywords=[]string{"invoice"};return s}(),func()Incoming{i:=incoming;i.Text="";i.MediaType="photo";return i}(),false},
 }
 for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){if got:=tt.settings.Allows(tt.incoming);got!=tt.want{t.Fatalf("Allows()=%v want %v",got,tt.want)}}) }
}

func TestValidateSettings(t *testing.T) {
 validID := "8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234"
 tests := []struct{name string;settings CollectionSettings;wantErr bool}{
  {"normalizes nil slices",CollectionSettings{Enabled:true,IncludeAllAccounts:true,IncludeText:true,IncludeMedia:true},false},
  {"rejects malformed account id",CollectionSettings{Enabled:true,AccountIDs:[]string{"bad"}},true},
  {"rejects duplicate account id",CollectionSettings{Enabled:true,AccountIDs:[]string{validID,validID}},true},
  {"rejects duplicate chat id",CollectionSettings{Enabled:true,ChatIDs:[]int64{42,42}},true},
  {"rejects zero chat id",CollectionSettings{Enabled:true,ChatIDs:[]int64{0}},true},
  {"rejects blank keyword",CollectionSettings{Enabled:true,Keywords:[]string{"  "}},true},
  {"rejects oversized keyword",CollectionSettings{Enabled:true,Keywords:[]string{strings.Repeat("x",65)}},true},
 }
 for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){_,err:=validateSettings(tt.settings);if (err!=nil)!=tt.wantErr{t.Fatalf("validateSettings() error=%v wantErr=%v",err,tt.wantErr)}}) }
}
