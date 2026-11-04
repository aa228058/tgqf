package messages

import (
	"strings"
	"testing"
)

func TestParseGroupCatalogueCSVAcceptsExportedColumns(t *testing.T) {
	input := "id,telegram_chat_id,title,chat_type,username,member_count,is_verified\n42,-1001234567890,Design Group,channel,design_group,1200,true\n"
	records, err := parseGroupCatalogueCSV(strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if len(records) != 1 { t.Fatalf("records=%d, want 1",len(records)) }
	got := records[0]
	if got.ChatID != -1001234567890 || got.ChatType != "channel" {
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


func TestParseGroupCatalogueTXTUsesTabDelimitedColumns(t *testing.T) {
	input := "telegram_chat_id\tchat_type\ttitle\tusername\tmember_count\tis_verified\n-1001234567890\tgroup\tOps Group\tops_group\t42\tfalse\n"
	records, err := parseGroupCatalogueTXT(strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if len(records) != 1 { t.Fatalf("records=%d, want 1",len(records)) }
	got := records[0]
	if got.ChatID != -1001234567890 || got.ChatType != "group" || got.Title != "Ops Group" || got.Username != "ops_group" || got.MemberCount != 42 || got.IsVerified {
		t.Fatalf("unexpected TXT import record: %#v",got)
	}
}

func TestParseGroupCatalogueTXTRejectsCommaSeparatedInput(t *testing.T) {
	input := "telegram_chat_id,chat_type,title\n123,group,Name\n"
	if _, err := parseGroupCatalogueTXT(strings.NewReader(input)); err == nil {
		t.Fatal("TXT importer must require tab-delimited columns")
	}
}


func TestGroupCatalogueXLSXRoundTrip(t *testing.T) {
	rows:=[][]string{
		{"id","telegram_chat_id","title","chat_type","username","member_count","is_verified"},
		{"42","-1001234567890","Research & Development","channel","research_news","1200","true"},
		{"43","-1001234567891","Second Group","group","","0","false"},
	}
	data,err:=buildGroupCatalogueXLSX(rows)
	if err!=nil{t.Fatal(err)}
	records,err:=parseGroupCatalogueXLSX(data)
	if err!=nil{t.Fatal(err)}
	if len(records)!=2{t.Fatalf("records=%d, want 2",len(records))}
	if records[0].ChatID!=-1001234567890||records[0].Title!="Research & Development"||records[0].Username!="research_news"||!records[0].IsVerified{
		t.Fatalf("unexpected first XLSX record: %#v",records[0])
	}
	if records[1].ChatID!=-1001234567891||records[1].ChatType!="group"{t.Fatalf("unexpected second XLSX record: %#v",records[1])}
}

func TestGroupCatalogueXLSXRejectsInvalidArchive(t *testing.T) {
	if _,err:=parseGroupCatalogueXLSX([]byte("not an xlsx workbook"));err==nil{t.Fatal("expected invalid XLSX archive error")}
}
