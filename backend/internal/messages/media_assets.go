package messages

import (
    "context"
    "crypto/sha256"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"
)

const MaxKeywordMediaAssetBytes = 20 << 20

var ErrMediaAssetNotFound = errors.New("media asset not found")

type MediaAsset struct {
    ID string `json:"id"`
    FileName string `json:"file_name"`
    MIMEType string `json:"mime_type"`
    FileSize int64 `json:"file_size"`
    SHA256 string `json:"sha256"`
    CreatedAt time.Time `json:"created_at"`
}

type MediaAssetContent struct {
    MediaAsset
    Content []byte
}

func (s *Service) CreateMediaAsset(ctx context.Context, tenantID, fileName, mimeType string, content []byte) (MediaAsset,error) {
    var out MediaAsset
    fileName=strings.TrimSpace(fileName)
    mimeType=strings.ToLower(strings.TrimSpace(mimeType))
    if tenantID=="" || fileName=="" || len(fileName)>255 || mimeType=="" || len(mimeType)>160 || len(content)==0 || len(content)>MaxKeywordMediaAssetBytes {
        return out,fmt.Errorf("invalid media asset metadata or size")
    }
    digest:=sha256.Sum256(content)
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return out,err};defer tx.Rollback()
    if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return out,err}
    err=tx.QueryRowContext(ctx,`
        INSERT INTO tenant_media_assets(tenant_id,file_name,mime_type,file_size,sha256,content)
        VALUES($1,$2,$3,$4,$5,$6)
        ON CONFLICT(tenant_id,sha256) DO UPDATE SET file_name=EXCLUDED.file_name,mime_type=EXCLUDED.mime_type
        RETURNING id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at`,
        tenantID,fileName,mimeType,len(content),digest[:]).Scan(&out.ID,&out.FileName,&out.MIMEType,&out.FileSize,&out.SHA256,&out.CreatedAt)
    if err!=nil{return out,fmt.Errorf("store media asset: %w",err)}
    if err=tx.Commit();err!=nil{return out,err}
    return out,nil
}

func (s *Service) ListMediaAssets(ctx context.Context,tenantID string,limit int)([]MediaAsset,error) {
    if limit<1||limit>200{limit=100}
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback()
    if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return nil,err}
    rows,err:=tx.QueryContext(ctx,`SELECT id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at FROM tenant_media_assets WHERE tenant_id=$1 ORDER BY created_at DESC,id LIMIT $2`,tenantID,limit)
    if err!=nil{return nil,err};defer rows.Close()
    items:=[]MediaAsset{}
    for rows.Next(){var item MediaAsset;if err:=rows.Scan(&item.ID,&item.FileName,&item.MIMEType,&item.FileSize,&item.SHA256,&item.CreatedAt);err!=nil{return nil,err};items=append(items,item)}
    if err:=rows.Err();err!=nil{return nil,err}
    if err:=tx.Commit();err!=nil{return nil,err}
    return items,nil
}

func (s *Service) GetMediaAsset(ctx context.Context,tenantID,id string)(MediaAssetContent,error) {
    var out MediaAssetContent
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return out,err};defer tx.Rollback()
    if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return out,err}
    err=tx.QueryRowContext(ctx,`SELECT id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at,content FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2`,tenantID,id).Scan(&out.ID,&out.FileName,&out.MIMEType,&out.FileSize,&out.SHA256,&out.CreatedAt,&out.Content)
    if errors.Is(err,sql.ErrNoRows){return out,ErrMediaAssetNotFound};if err!=nil{return out,err}
    if err:=tx.Commit();err!=nil{return out,err}
    return out,nil
}

func (s *Service) DeleteMediaAsset(ctx context.Context,tenantID,id string)error {
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
    if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return err}
    result,err:=tx.ExecContext(ctx,`DELETE FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2`,tenantID,id);if err!=nil{return err}
    count,err:=result.RowsAffected();if err!=nil{return err};if count==0{return ErrMediaAssetNotFound}
    return tx.Commit()
}
