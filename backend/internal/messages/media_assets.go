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
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	MIMEType  string    `json:"mime_type"`
	FileSize  int64     `json:"file_size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type MediaAssetContent struct {
	MediaAsset
	Content []byte
}

func (s *Service) CreateMediaAsset(ctx context.Context, tenantID, fileName, mimeType string, content []byte) (MediaAsset, error) {
	var out MediaAsset
	fileName = strings.TrimSpace(fileName)
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if tenantID == "" || fileName == "" || len(fileName) > 255 || mimeType == "" || len(mimeType) > 160 || len(content) == 0 || len(content) > MaxKeywordMediaAssetBytes {
		return out, fmt.Errorf("invalid media asset metadata or size")
	}
	digest := sha256.Sum256(content)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `
        INSERT INTO tenant_media_assets(tenant_id,file_name,mime_type,file_size,sha256,content)
        VALUES($1,$2,$3,$4,$5,$6)
        ON CONFLICT(tenant_id,sha256) DO UPDATE SET file_name=EXCLUDED.file_name,mime_type=EXCLUDED.mime_type
        RETURNING id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at`,
		tenantID, fileName, mimeType, len(content), digest[:], content).Scan(&out.ID, &out.FileName, &out.MIMEType, &out.FileSize, &out.SHA256, &out.CreatedAt)
	if err != nil {
		return out, fmt.Errorf("store media asset: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) ListMediaAssets(ctx context.Context, tenantID string, limit int) ([]MediaAsset, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at FROM tenant_media_assets WHERE tenant_id=$1 ORDER BY created_at DESC,id LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MediaAsset{}
	for rows.Next() {
		var item MediaAsset
		if err := rows.Scan(&item.ID, &item.FileName, &item.MIMEType, &item.FileSize, &item.SHA256, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) GetMediaAsset(ctx context.Context, tenantID, id string) (MediaAssetContent, error) {
	var out MediaAssetContent
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT id::text,file_name,mime_type,file_size,encode(sha256,'hex'),created_at,content FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&out.ID, &out.FileName, &out.MIMEType, &out.FileSize, &out.SHA256, &out.CreatedAt, &out.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrMediaAssetNotFound
	}
	if err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) DeleteMediaAsset(ctx context.Context, tenantID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrMediaAssetNotFound
	}
	return tx.Commit()
}

type CachedMessageMedia struct {
	MediaAsset
	Content []byte
}

// CacheMessageMedia stores downloaded Telegram bytes in the tenant's existing
// SHA-256 deduplicated asset store, then links that asset to its source message.
func (s *Service) CacheMessageMedia(ctx context.Context, tenantID, accountID string, chatID, messageID int64, fileName, mimeType string, content []byte) (MediaAsset, error) {
	asset, err := s.CreateMediaAsset(ctx, tenantID, fileName, mimeType, content)
	if err != nil {
		return MediaAsset{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaAsset{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return MediaAsset{}, err
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO message_media_cache(tenant_id,telegram_account_id,chat_id,message_id,asset_id,cached_at)
        VALUES($1,$2,$3,$4,$5,NOW())
        ON CONFLICT(tenant_id,telegram_account_id,chat_id,message_id)
        DO UPDATE SET asset_id=EXCLUDED.asset_id,cached_at=NOW()`,
		tenantID, accountID, chatID, messageID, asset.ID)
	if err != nil {
		return MediaAsset{}, err
	}
	if err = tx.Commit(); err != nil {
		return MediaAsset{}, err
	}
	return asset, nil
}

// GetCachedMessageMedia only resolves a cache entry within the caller's tenant.
func (s *Service) GetCachedMessageMedia(ctx context.Context, tenantID, accountID string, chatID, messageID int64) (CachedMessageMedia, error) {
	var out CachedMessageMedia
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `
        SELECT a.id::text,a.file_name,a.mime_type,a.file_size,encode(a.sha256,'hex'),a.created_at,a.content
        FROM message_media_cache c
        JOIN tenant_media_assets a ON a.id=c.asset_id AND a.tenant_id=c.tenant_id
        WHERE c.tenant_id=$1 AND c.telegram_account_id=$2 AND c.chat_id=$3 AND c.message_id=$4`,
		tenantID, accountID, chatID, messageID).Scan(&out.ID, &out.FileName, &out.MIMEType, &out.FileSize, &out.SHA256, &out.CreatedAt, &out.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrMediaAssetNotFound
	}
	if err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

// PruneMessageMediaCache expires source-message cache links for one tenant.
// The deduplicated asset rows are intentionally retained: they may also be
// referenced by keyword actions or the tenant's reusable media library.
func (s *Service) PruneMessageMediaCache(ctx context.Context, tenantID string, retention time.Duration) (int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return 0, errors.New("tenant scope is required")
	}
	if retention < 24*time.Hour {
		retention = 90 * 24 * time.Hour
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM message_media_cache WHERE tenant_id=$1 AND cached_at < NOW()-($2 * INTERVAL '1 second')`, tenantID, int64(retention.Seconds()))
	if err != nil {
		return 0, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}
