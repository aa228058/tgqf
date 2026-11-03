package accounts

import (
    "bytes"
    "encoding/csv"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "path/filepath"
    "strings"

    "github.com/aa228058/tgqf/backend/internal/auth"
)

const maxSessionImportFileBytes = 1 << 20

func parseAccountImportFile(filename string,data []byte)([]createRequest,error) {
    if len(data)==0||len(data)>maxSessionImportFileBytes{return nil,errors.New("invalid import file size")}
    switch strings.ToLower(filepath.Ext(filename)) {
    case ".json":
        decoder:=json.NewDecoder(bytes.NewReader(data));decoder.DisallowUnknownFields()
        var input batchImportRequest
        if err:=decoder.Decode(&input);err!=nil{return nil,errors.New("invalid JSON import file")}
        var extra any
        if err:=decoder.Decode(&extra);err!=io.EOF{return nil,errors.New("JSON file must contain exactly one value")}
        if len(input.Accounts)==0||len(input.Accounts)>20{return nil,errors.New("import file must contain between 1 and 20 accounts")}
        return input.Accounts,nil
    case ".csv":
        reader:=csv.NewReader(bytes.NewReader(data));reader.FieldsPerRecord=-1
        header,err:=reader.Read();if err!=nil{return nil,errors.New("CSV header is required")}
        indexes:=map[string]int{}
        for i,name:=range header {
            name=strings.TrimSpace(strings.TrimPrefix(name,"\ufeff"))
            if name!="phone"&&name!="display_name"&&name!="group_name"&&name!="session"{return nil,errors.New("CSV contains an unsupported column")}
            if _,exists:=indexes[name];exists{return nil,errors.New("CSV contains duplicate columns")}
            indexes[name]=i
        }
        if _,ok:=indexes["phone"];!ok{return nil,errors.New("CSV phone column is required")}
        if _,ok:=indexes["session"];!ok{return nil,errors.New("CSV session column is required")}
        records:=[]createRequest{}
        for {
            row,err:=reader.Read()
            if err==io.EOF{break};if err!=nil{return nil,errors.New("invalid CSV row")}
            if len(row)!=len(header){return nil,errors.New("CSV row has an inconsistent number of columns")}
            value:=func(key string)string{if i,ok:=indexes[key];ok{return strings.TrimSpace(row[i])};return ""}
            records=append(records,createRequest{Phone:value("phone"),DisplayName:value("display_name"),GroupName:value("group_name"),Session:value("session")})
            if len(records)>20{return nil,errors.New("import file exceeds 20 accounts")}
        }
        if len(records)==0{return nil,errors.New("CSV contains no account records")}
        return records,nil
    case ".txt":
        records:=[]createRequest{}
        for _,line:=range strings.Split(strings.ReplaceAll(string(data),"\r\n","\n"),"\n") {
            line=strings.TrimSpace(line);if line==""{continue}
            fields:=strings.Split(line,"\t")
            item:=createRequest{}
            switch len(fields) {
            case 2:
                item.Phone=strings.TrimSpace(fields[0]);item.Session=strings.TrimSpace(fields[1])
            case 3:
                item.Phone=strings.TrimSpace(fields[0]);item.DisplayName=strings.TrimSpace(fields[1]);item.Session=strings.TrimSpace(fields[2])
            case 4:
                item.Phone=strings.TrimSpace(fields[0]);item.DisplayName=strings.TrimSpace(fields[1]);item.GroupName=strings.TrimSpace(fields[2]);item.Session=strings.TrimSpace(fields[3])
            default:
                return nil,errors.New("TXT lines must contain phone and session separated by tabs")
            }
            records=append(records,item);if len(records)>20{return nil,errors.New("import file exceeds 20 accounts")}
        }
        if len(records)==0{return nil,errors.New("TXT contains no account records")}
        return records,nil
    default:
        return nil,errors.New("supported import formats are .json, .csv and .txt")
    }
}

func (h *Handler) importFile(w http.ResponseWriter,r *http.Request) {
    r.Body=http.MaxBytesReader(w,r.Body,2*maxSessionImportFileBytes)
    if err:=r.ParseMultipartForm(1<<20);err!=nil{writeError(w,http.StatusBadRequest,"invalid multipart upload or file too large");return}
    defer func(){if r.MultipartForm!=nil{_=r.MultipartForm.RemoveAll()}}()
    file,header,err:=r.FormFile("file")
    if err!=nil{writeError(w,http.StatusBadRequest,"file field is required");return}
    defer file.Close()
    data,err:=io.ReadAll(io.LimitReader(file,maxSessionImportFileBytes+1))
    if err!=nil||len(data)>maxSessionImportFileBytes{writeError(w,http.StatusBadRequest,"import file exceeds 1 MiB");return}
    records,err:=parseAccountImportFile(header.Filename,data)
    if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
    h.importAccounts(w,r,records)
}

func (h *Handler) importAccounts(w http.ResponseWriter,r *http.Request,records []createRequest) {
    if len(records)==0||len(records)>20{writeError(w,http.StatusBadRequest,"import must contain between 1 and 20 accounts");return}
    u:=auth.UserFromContext(r.Context())
    type itemResult struct{Index int `json:"index"`;Account *Account `json:"account,omitempty"`;Error string `json:"error,omitempty"`}
    results:=make([]itemResult,0,len(records));success:=0
    for i,item:=range records {
        account,err:=h.service.Create(r.Context(),u.TenantID,item.Phone,item.DisplayName,item.GroupName,item.Session,item.Proxy)
        if err!=nil {
            message:="account import failed"
            if strings.Contains(err.Error(),"account limit reached") {
                message="account limit reached";results=append(results,itemResult{Index:i,Error:message})
                for j:=i+1;j<len(records);j++{results=append(results,itemResult{Index:j,Error:"not processed after account limit reached"})}
                break
            }
            if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){message="session encryption is not configured"}
            results=append(results,itemResult{Index:i,Error:message});continue
        }
        results=append(results,itemResult{Index:i,Account:&account});success++
    }
    status:=http.StatusOK
    if success==0{status=http.StatusUnprocessableEntity}else if success<len(records){status=http.StatusMultiStatus}
    writeJSON(w,status,map[string]any{"total":len(records),"succeeded":success,"failed":len(records)-success,"results":results})
}
