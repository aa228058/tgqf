package accounts

import (
    "strings"
    "testing"
)

func TestParseAccountImportFileCSV(t *testing.T) {
    records,err:=parseAccountImportFile("accounts.csv",[]byte("phone,display_name,group_name,session\n+15550000001,Example,team,SESSION_ONE\n"))
    if err!=nil{t.Fatal(err)}
    if len(records)!=1||records[0].Phone!="+15550000001"||records[0].Session!="SESSION_ONE"||records[0].GroupName!="team"{t.Fatalf("unexpected parsed records: %+v",records)}
}

func TestParseAccountImportFileTXT(t *testing.T) {
    records,err:=parseAccountImportFile("accounts.txt",[]byte("+15550000001\tExample\tteam\tSESSION_ONE\n+15550000002\tSESSION_TWO\n"))
    if err!=nil{t.Fatal(err)}
    if len(records)!=2||records[0].DisplayName!="Example"||records[1].Session!="SESSION_TWO"{t.Fatalf("unexpected parsed records: %+v",records)}
}

func TestParseAccountImportFileRejectsUnsupportedOrOversizedFiles(t *testing.T) {
    if _,err:=parseAccountImportFile("accounts.session",[]byte("not a supported MTGo export"));err==nil{t.Fatal("expected unsupported extension error")}
    if _,err:=parseAccountImportFile("accounts.txt",[]byte(strings.Repeat("x",maxSessionImportFileBytes+1)));err==nil{t.Fatal("expected oversized file error")}
}

func TestParseAccountImportFileRejectsBadCSVHeaders(t *testing.T) {
    _,err:=parseAccountImportFile("accounts.csv",[]byte("phone,session,password\n+15550000001,SESSION_ONE,secret\n"))
    if err==nil{t.Fatal("expected unsupported column error")}
}

func TestParseAccountImportFileJSON(t *testing.T) {
    records,err:=parseAccountImportFile("accounts.json",[]byte(`{"accounts":[{"phone":"+15550000001","display_name":"Example","session":"SESSION_ONE"}]}`))
    if err!=nil{t.Fatal(err)}
    if len(records)!=1||records[0].Phone!="+15550000001"||records[0].Session!="SESSION_ONE"{t.Fatalf("unexpected parsed records: %+v",records)}
}

func TestParseAccountImportFileRejectsMoreThanTwentyRecords(t *testing.T) {
    var input strings.Builder
    input.WriteString("phone,session\n")
    for i:=0;i<21;i++{input.WriteString("+15550000000,SESSION\n")}
    if _,err:=parseAccountImportFile("accounts.csv",[]byte(input.String()));err==nil{t.Fatal("expected account count limit error")}
}
