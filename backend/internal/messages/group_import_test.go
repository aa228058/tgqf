package messages

import (
	"strings"
	"testing"
)

func TestParseGroupCatalogueCSVAcceptsExportedColumns(t *testing.T) {
	input := "id,telegram_chat_id,title,chat_type,username,member_count,is_verified\n42,-1001234567890,Design Group,supergroup,design_group,1200,true\n"
	records, err := parseGroupCatalogueCSV(strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if len(records) != 1 { t.Fatalf("records=%d, want 1",len(records)) }
	got := records[0]
	if got.ChatID != -1001234567890 || got.ChatType != "supergroup" {
		// The catalogue intentionally stores Telegram's normalized channel
		// entity kind as "channel"; imported files must use supported kinds.
		t.Fatalf("unexpected identity: %#v",got)
	}
}

func TestParseGroupCatalogueCSVRejectsInvalidAndDuplicateHeaders(t *testing.T) {
	tests := []struct{name,input string}{
		{"duplicate header","chat_id,chat_id,chat_type,title\n1,2,group,Name\n"},
		{"invalid chat id","chat_id,chat_type,title\nnot-a-number,group,Name\n"},
		{"unsupported chat type","chat_id,chat_type,title\n123,supergroup,Name\n"},
		{"overlong username","chat_id,chat_type,title,username\n123,group,Name,abcdefghijklmnopqrstuvwxyz123456789\n"},
	}
	for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){
		if _,err:=parseGroupCatalogueCSV(strings.NewReader(tt.input));err==nil{t.Fatal("expected validation error")}
	})}
}

func TestNormalizeGroupCatalogueImportRecord(t *testing.T) {
	got,err:=normalizeGroupCatalogueImportRecord(GroupCatalogueImportRecord{
		ChatID:-1001234567890,ChatType:" CHANNEL ",Title:"  News  ",Username:"@news_room",MemberCount:10,
	})
	if err!=nil{t.Fatal(err)}
	if got.ChatType!="channel"||got.Title!="News"||got.Username!="news_room"{t.Fatalf("not normalized: %#v",got)}
}
