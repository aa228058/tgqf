package accounts

import (
    "database/sql"
    "os"
    "strings"
    "testing"
    tgconv "github.com/mtgo-labs/session-converter"
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

func makeTelethonSessionSQLite(t *testing.T, rows int) []byte {
    t.Helper()
    tmp,err:=os.CreateTemp(t.TempDir(),"telethon-*.session")
    if err!=nil{t.Fatal(err)}
    path:=tmp.Name()
    if err:=tmp.Close();err!=nil{t.Fatal(err)}
    db,err:=sql.Open("sqlite",path)
    if err!=nil{t.Fatal(err)}
    if _,err=db.Exec(`CREATE TABLE sessions (dc_id INTEGER, server_address TEXT, port INTEGER, auth_key BLOB, takeout_id INTEGER)`);err!=nil{t.Fatal(err)}
    key:=make([]byte,256);for i:=range key{key[i]=byte(i+1)}
    for i:=0;i<rows;i++{if _,err=db.Exec(`INSERT INTO sessions(dc_id,server_address,port,auth_key) VALUES(?,?,?,?)`,4,"149.154.167.50",443,key);err!=nil{t.Fatal(err)}}
    if err:=db.Close();err!=nil{t.Fatal(err)}
    data,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
    return data
}

func TestParseTelethonSQLiteSession(t *testing.T) {
    encoded,err:=parseTelethonSQLiteSession(makeTelethonSessionSQLite(t,1))
    if err!=nil{t.Fatal(err)}
    decoded,format,err:=tgconv.Decode(encoded)
    if err!=nil{t.Fatal(err)}
    if format!=tgconv.FormatTelethon||decoded.DCID!=4||decoded.ServerAddress!="149.154.167.50"||decoded.Port!=443||len(decoded.AuthKey)!=256{t.Fatalf("unexpected decoded Telethon session: format=%s session=%+v",format,decoded)}
}

func TestParseTelethonSQLiteSessionRejectsInvalidDatabases(t *testing.T) {
    if _,err:=parseTelethonSQLiteSession([]byte("not sqlite"));err==nil{t.Fatal("expected invalid SQLite rejection")}
    if _,err:=parseTelethonSQLiteSession(makeTelethonSessionSQLite(t,2));err==nil{t.Fatal("expected multi-session database rejection")}
}

func TestParseAccountImportFileTelethonSession(t *testing.T) {
    records,err:=parseAccountImportFile("account.session",makeTelethonSessionSQLite(t,1))
    if err!=nil{t.Fatal(err)}
    if len(records)!=1||records[0].Session==""||records[0].Phone!=""{t.Fatalf("unexpected imported record: %+v",records)}
}
