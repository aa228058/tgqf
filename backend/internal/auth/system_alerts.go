package auth

import (
    "context"
    "encoding/json"
    "fmt"
)

type SystemAlert struct {
    ID int64 `json:"id"`
    Key string `json:"key"`
    Severity string `json:"severity"`
    Status string `json:"status"`
    Title string `json:"title"`
    Details map[string]any `json:"details"`
    OpenedAt string `json:"opened_at"`
    LastSeenAt string `json:"last_seen_at"`
    ResolvedAt *string `json:"resolved_at,omitempty"`
}

type alertCondition struct { key,severity,title string; details map[string]any }

func (s *Service) SyncSystemAlerts(ctx context.Context, health SystemHealth) error {
    conditions:=make([]alertCondition,0)
    if health.Database.Status!="ok" { conditions=append(conditions,alertCondition{"postgresql_unavailable","critical","PostgreSQL 连接异常",map[string]any{"status":health.Database.Status}}) }
    if health.Redis.Status!="ok" { conditions=append(conditions,alertCondition{"redis_unavailable","critical","Redis 连接异常",map[string]any{"status":health.Redis.Status}}) }
    if health.Host.DiskTotalBytes>0 && health.Host.DiskUsedPercent>=90 { conditions=append(conditions,alertCondition{"disk_critical","critical","磁盘使用率达到 90%",map[string]any{"used_percent":health.Host.DiskUsedPercent}}) } else if health.Host.DiskTotalBytes>0 && health.Host.DiskUsedPercent>=85 { conditions=append(conditions,alertCondition{"disk_warning","warning","磁盘使用率达到 85%",map[string]any{"used_percent":health.Host.DiskUsedPercent}}) }
    if health.Host.CPUPercent>=90 { conditions=append(conditions,alertCondition{"cpu_critical","critical","CPU 使用率达到 90%",map[string]any{"used_percent":health.Host.CPUPercent}}) }
    if health.Workers.Stale>0 { conditions=append(conditions,alertCondition{"worker_stale","critical","存在 Worker 心跳超时",map[string]any{"stale":health.Workers.Stale}}) }
    if health.Accounts.Error>0 { conditions=append(conditions,alertCondition{"telegram_account_errors","warning","Telegram 账号存在错误状态",map[string]any{"count":health.Accounts.Error}}) }
    if health.Proxies.FailedChecks15m>=5 { conditions=append(conditions,alertCondition{"proxy_failures","warning","代理近 15 分钟检查失败次数过多",map[string]any{"failed_checks":health.Proxies.FailedChecks15m}}) }
    if health.Queues.IncomingPending>=100 { conditions=append(conditions,alertCondition{"incoming_queue_backlog","warning","Telegram 入站队列积压",map[string]any{"pending":health.Queues.IncomingPending}}) }
    if health.Queues.MessageOutboxPending>=1000 { conditions=append(conditions,alertCondition{"message_outbox_backlog","warning","消息事件 Outbox 积压",map[string]any{"pending":health.Queues.MessageOutboxPending}}) }
    active:=make(map[string]alertCondition,len(conditions))
    for _,condition:=range conditions { active[condition.key]=condition }
    tx,err:=s.db.BeginTx(ctx,nil)
    if err!=nil{return err}
    defer tx.Rollback()
    for _,condition:=range conditions {
        details,marshalErr:=json.Marshal(condition.details)
        if marshalErr!=nil{return marshalErr}
        _,err=tx.ExecContext(ctx,`INSERT INTO system_alerts(alert_key,severity,status,title,details)
            VALUES($1,$2,'active',$3,$4::jsonb)
            ON CONFLICT(alert_key) WHERE status='active'
            DO UPDATE SET severity=EXCLUDED.severity,title=EXCLUDED.title,details=EXCLUDED.details,last_seen_at=NOW()`,
            condition.key,condition.severity,condition.title,string(details))
        if err!=nil{return fmt.Errorf("upsert system alert %s: %w",condition.key,err)}
    }
    rows,err:=tx.QueryContext(ctx,`SELECT DISTINCT alert_key FROM system_alerts WHERE status='active'`)
    if err!=nil{return err}
    var stale []string
    for rows.Next(){var key string;if err:=rows.Scan(&key);err!=nil{rows.Close();return err};if _,ok:=active[key];!ok{stale=append(stale,key)}}
    if err:=rows.Err();err!=nil{rows.Close();return err}
    rows.Close()
    for _,key:=range stale { if _,err=tx.ExecContext(ctx,`UPDATE system_alerts SET status='resolved',resolved_at=NOW() WHERE alert_key=$1 AND status='active'`,key);err!=nil{return err} }
    return tx.Commit()
}

func (s *Service) ListSystemAlerts(ctx context.Context, limit int) ([]SystemAlert,error) {
    if limit<1||limit>200 {limit=100}
    rows,err:=s.db.QueryContext(ctx,`SELECT id,alert_key,severity,status,title,details::text,opened_at::text,last_seen_at::text,resolved_at::text
        FROM system_alerts ORDER BY CASE status WHEN 'active' THEN 0 ELSE 1 END,opened_at DESC LIMIT $1`,limit)
    if err!=nil{return nil,err}
    defer rows.Close()
    alerts:=make([]SystemAlert,0)
    for rows.Next(){var item SystemAlert;var details string;var resolved *string;if err:=rows.Scan(&item.ID,&item.Key,&item.Severity,&item.Status,&item.Title,&details,&item.OpenedAt,&item.LastSeenAt,&resolved);err!=nil{return nil,err};if err:=json.Unmarshal([]byte(details),&item.Details);err!=nil{return nil,err};item.ResolvedAt=resolved;alerts=append(alerts,item)}
    return alerts,rows.Err()
}
