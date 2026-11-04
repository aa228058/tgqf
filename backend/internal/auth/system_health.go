package auth

import (
    "context"
	"os"
	"strconv"
	"strings"
	"sync"
    "fmt"
    "runtime"
    "syscall"
    "time"

)

type SystemHealth struct {
    Redis RedisHealth `json:"redis"`
    Queues QueueHealth `json:"queues"`
    CheckedAt time.Time `json:"checked_at"`
    Host HostHealth `json:"host"`
    Database DatabaseHealth `json:"database"`
    Workers WorkerHealth `json:"workers"`
    Accounts AccountHealth `json:"accounts"`
    Proxies ProxyHealth `json:"proxies"`
}
type HostHealth struct {
    CPUPercent float64 `json:"cpu_percent"`
    Goroutines int `json:"goroutines"`
    HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
    HeapInUseBytes uint64 `json:"heap_in_use_bytes"`
    SysBytes uint64 `json:"sys_bytes"`
    DiskTotalBytes uint64 `json:"disk_total_bytes,omitempty"`
    DiskFreeBytes uint64 `json:"disk_free_bytes,omitempty"`
    DiskUsedPercent float64 `json:"disk_used_percent,omitempty"`
}
type DatabaseHealth struct {
    Status string `json:"status"`
    OpenConnections int `json:"open_connections"`
    InUseConnections int `json:"in_use_connections"`
    IdleConnections int `json:"idle_connections"`
    MaxOpenConnections int `json:"max_open_connections"`
    PingLatencyMS int64 `json:"ping_latency_ms"`
}
type RedisHealth struct { Status string `json:"status"`; PingLatencyMS int64 `json:"ping_latency_ms"` }
type QueueHealth struct { IncomingStreamLength int64 `json:"incoming_stream_length"`; IncomingPending int64 `json:"incoming_pending"`; MessageOutboxPending int64 `json:"message_outbox_pending"` }
type WorkerHealth struct {
    Total int `json:"total"`
    Running int `json:"running"`
    Stale int `json:"stale"`
    Stopped int `json:"stopped"`
}
type AccountHealth struct {
    Total int `json:"total"`
    Authorized int `json:"authorized"`
    Disconnected int `json:"disconnected"`
    Error int `json:"error"`
    Pending int `json:"pending"`
}
type ProxyHealth struct {
    Total int `json:"total"`
    FailedChecks15m int `json:"failed_checks_15m"`
    SuccessfulChecks15m int `json:"successful_checks_15m"`
}

func (s *Service) SystemHealth(ctx context.Context) (SystemHealth,error) {
    result:=SystemHealth{CheckedAt:time.Now().UTC()}
    var mem runtime.MemStats
    runtime.ReadMemStats(&mem)
    result.Host=HostHealth{CPUPercent:sampleCPUPercent(),Goroutines:runtime.NumGoroutine(),HeapAllocBytes:mem.HeapAlloc,HeapInUseBytes:mem.HeapInuse,SysBytes:mem.Sys}
    var fs syscall.Statfs_t
    if err:=syscall.Statfs("/",&fs);err==nil && fs.Blocks>0 {
        result.Host.DiskTotalBytes=fs.Blocks*uint64(fs.Bsize)
        result.Host.DiskFreeBytes=fs.Bavail*uint64(fs.Bsize)
        if result.Host.DiskTotalBytes>0 { result.Host.DiskUsedPercent=100*float64(result.Host.DiskTotalBytes-result.Host.DiskFreeBytes)/float64(result.Host.DiskTotalBytes) }
    }
    started:=time.Now()
    pingCtx,cancel:=context.WithTimeout(ctx,2*time.Second)
    pingErr:=s.db.PingContext(pingCtx)
    cancel()
    result.Database.Status="ok"
    if pingErr!=nil { result.Database.Status="error" }
    result.Database.PingLatencyMS=time.Since(started).Milliseconds()
    stats:=s.db.Stats()
    result.Database.OpenConnections=stats.OpenConnections
    result.Database.InUseConnections=stats.InUse
    result.Database.IdleConnections=stats.Idle
    result.Database.MaxOpenConnections=stats.MaxOpenConnections
    if err:=s.db.QueryRowContext(ctx,`SELECT COUNT(*),COUNT(*) FILTER(WHERE status='running' AND heartbeat_at >= NOW()-INTERVAL '45 seconds'),COUNT(*) FILTER(WHERE status='running' AND heartbeat_at < NOW()-INTERVAL '45 seconds'),COUNT(*) FILTER(WHERE status='stopped') FROM worker_instances`).Scan(&result.Workers.Total,&result.Workers.Running,&result.Workers.Stale,&result.Workers.Stopped);err!=nil{return result,fmt.Errorf("worker health: %w",err)}
    if err:=s.db.QueryRowContext(ctx,"SELECT COUNT(*) FROM message_event_outbox WHERE published_at IS NULL").Scan(&result.Queues.MessageOutboxPending);err!=nil{return result,fmt.Errorf("message outbox health: %w",err)}
    if s.redis==nil { result.Redis.Status="unavailable" } else {
        redisStarted:=time.Now()
        redisCtx,redisCancel:=context.WithTimeout(ctx,2*time.Second)
        redisErr:=s.redis.Ping(redisCtx).Err()
        redisCancel()
        result.Redis.PingLatencyMS=time.Since(redisStarted).Milliseconds()
        if redisErr!=nil { result.Redis.Status="error" } else {
            result.Redis.Status="ok"
            length,streamErr:=s.redis.XLen(ctx,"tgqf:telegram:incoming-updates").Result()
            if streamErr==nil { result.Queues.IncomingStreamLength=length }
            pending,pendingErr:=s.redis.XPending(ctx,"tgqf:telegram:incoming-updates","tgqf-message-ingestors").Result()
            if pendingErr==nil && pending!=nil { result.Queues.IncomingPending=pending.Count }
        }
    }
    if err:=s.aggregateTenantHealth(ctx,&result);err!=nil{return result,fmt.Errorf("tenant-scoped health: %w",err)}
    if pingErr!=nil{return result,fmt.Errorf("PostgreSQL ping failed: %w",pingErr)}
    return result,nil
}

func (h SystemHealth) HasWarnings() bool {
    return h.Database.Status!="ok" || h.Redis.Status!="ok" || h.Workers.Stale>0 || h.Accounts.Error>0 || h.Proxies.FailedChecks15m>0 || h.Host.DiskUsedPercent>=85 || h.Host.CPUPercent>=90 || h.Queues.IncomingPending>100 || h.Queues.MessageOutboxPending>1000
}


var cpuSampleMu sync.Mutex
var previousCPUTotal uint64
var previousCPUIdle uint64

func sampleCPUPercent() float64 {
    data,err:=os.ReadFile("/proc/stat")
    if err!=nil{return 0}
    fields:=strings.Fields(strings.SplitN(string(data),"\n",2)[0])
    if len(fields)<5 || fields[0]!="cpu" { return 0 }
    var total,idle uint64
    for i,value:=range fields[1:] {
        n,parseErr:=strconv.ParseUint(value,10,64)
        if parseErr!=nil { return 0 }
        total+=n
        if i==3 || i==4 { idle+=n }
    }
    cpuSampleMu.Lock()
    defer cpuSampleMu.Unlock()
    defer func(){previousCPUTotal=total;previousCPUIdle=idle}()
    if previousCPUTotal==0 || total<=previousCPUTotal { return 0 }
    totalDelta:=total-previousCPUTotal
    idleDelta:=idle-previousCPUIdle
    if totalDelta==0 || idleDelta>totalDelta { return 0 }
    return 100*float64(totalDelta-idleDelta)/float64(totalDelta)
}

func (s *Service) aggregateTenantHealth(ctx context.Context, result *SystemHealth) error {
    tx,err:=s.db.BeginTx(ctx,nil)
    if err!=nil{return err}
    defer tx.Rollback()
    rows,err:=tx.QueryContext(ctx,"SELECT id::text FROM tenants ORDER BY id")
    if err!=nil{return err}
    tenantIDs:=make([]string,0)
    for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return err};tenantIDs=append(tenantIDs,id)}
    if err:=rows.Err();err!=nil{rows.Close();return err}
    rows.Close()
    for _,tenantID:=range tenantIDs {
        if _,err:=tx.ExecContext(ctx,"SELECT set_config('app.tenant_id',$1,true)",tenantID);err!=nil{return err}
        var total,authorized,disconnected,accountErrors,pending int
        if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*),COUNT(*) FILTER(WHERE status='authorized'),COUNT(*) FILTER(WHERE status='disconnected'),COUNT(*) FILTER(WHERE status='error'),COUNT(*) FILTER(WHERE status='pending') FROM telegram_accounts WHERE tenant_id=$1",tenantID).Scan(&total,&authorized,&disconnected,&accountErrors,&pending);err!=nil{return fmt.Errorf("account health for tenant: %w",err)}
        result.Accounts.Total+=total;result.Accounts.Authorized+=authorized;result.Accounts.Disconnected+=disconnected;result.Accounts.Error+=accountErrors;result.Accounts.Pending+=pending
        var proxies,failedChecks,successfulChecks int
        if err:=tx.QueryRowContext(ctx,"SELECT (SELECT COUNT(*) FROM tenant_proxies WHERE tenant_id=$1),(SELECT COUNT(*) FILTER(WHERE status='failed') FROM tenant_proxy_checks WHERE tenant_id=$1 AND checked_at >= NOW()-INTERVAL '15 minutes'),(SELECT COUNT(*) FILTER(WHERE status='success') FROM tenant_proxy_checks WHERE tenant_id=$1 AND checked_at >= NOW()-INTERVAL '15 minutes')",tenantID).Scan(&proxies,&failedChecks,&successfulChecks);err!=nil{return fmt.Errorf("proxy health for tenant: %w",err)}
        result.Proxies.Total+=proxies;result.Proxies.FailedChecks15m+=failedChecks;result.Proxies.SuccessfulChecks15m+=successfulChecks
    }
    return tx.Commit()
}
