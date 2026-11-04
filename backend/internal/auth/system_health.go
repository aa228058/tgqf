package auth

import (
    "context"
    "fmt"
    "runtime"
    "syscall"
    "time"
)

type SystemHealth struct {
    CheckedAt time.Time `json:"checked_at"`
    Host HostHealth `json:"host"`
    Database DatabaseHealth `json:"database"`
    Workers WorkerHealth `json:"workers"`
    Accounts AccountHealth `json:"accounts"`
    Proxies ProxyHealth `json:"proxies"`
}
type HostHealth struct {
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
    result.Host=HostHealth{Goroutines:runtime.NumGoroutine(),HeapAllocBytes:mem.HeapAlloc,HeapInUseBytes:mem.HeapInuse,SysBytes:mem.Sys}
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
    if err:=s.db.QueryRowContext(ctx,`SELECT COUNT(*),COUNT(*) FILTER(WHERE status='authorized'),COUNT(*) FILTER(WHERE status='disconnected'),COUNT(*) FILTER(WHERE status='error'),COUNT(*) FILTER(WHERE status='pending') FROM telegram_accounts`).Scan(&result.Accounts.Total,&result.Accounts.Authorized,&result.Accounts.Disconnected,&result.Accounts.Error,&result.Accounts.Pending);err!=nil{return result,fmt.Errorf("account health: %w",err)}
    if err:=s.db.QueryRowContext(ctx,`SELECT (SELECT COUNT(*) FROM tenant_proxies),(SELECT COUNT(*) FILTER(WHERE status='failed') FROM tenant_proxy_checks WHERE checked_at >= NOW()-INTERVAL '15 minutes'),(SELECT COUNT(*) FILTER(WHERE status='success') FROM tenant_proxy_checks WHERE checked_at >= NOW()-INTERVAL '15 minutes')`).Scan(&result.Proxies.Total,&result.Proxies.FailedChecks15m,&result.Proxies.SuccessfulChecks15m);err!=nil{return result,fmt.Errorf("proxy health: %w",err)}
    if pingErr!=nil{return result,fmt.Errorf("PostgreSQL ping failed: %w",pingErr)}
    return result,nil
}

func (h SystemHealth) HasWarnings() bool {
    return h.Database.Status!="ok" || h.Workers.Stale>0 || h.Accounts.Error>0 || h.Proxies.FailedChecks15m>0 || h.Host.DiskUsedPercent>=85
}

