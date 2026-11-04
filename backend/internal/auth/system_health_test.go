package auth

import (
    "testing"
    "time"
)

func TestSystemHealthHasWarnings(t *testing.T) {
    tests:=[]struct{name string;health SystemHealth;want bool}{
        {"healthy",SystemHealth{Database:DatabaseHealth{Status:"ok"},Host:HostHealth{DiskUsedPercent:50}},false},
        {"database error",SystemHealth{Database:DatabaseHealth{Status:"error"}},true},
        {"stale worker",SystemHealth{Database:DatabaseHealth{Status:"ok"},Workers:WorkerHealth{Stale:1}},true},
        {"account error",SystemHealth{Database:DatabaseHealth{Status:"ok"},Accounts:AccountHealth{Error:1}},true},
        {"proxy failure",SystemHealth{Database:DatabaseHealth{Status:"ok"},Proxies:ProxyHealth{FailedChecks15m:1}},true},
        {"disk warning threshold",SystemHealth{Database:DatabaseHealth{Status:"ok"},Host:HostHealth{DiskUsedPercent:85}},true},
    }
    for _,tt:=range tests{t.Run(tt.name,func(t *testing.T){if got:=tt.health.HasWarnings();got!=tt.want{t.Fatalf("HasWarnings()=%v want %v",got,tt.want)}})}
}

func TestSystemHealthCheckedAtIsUTC(t *testing.T) {
    now:=time.Now().UTC()
    health:=SystemHealth{CheckedAt:now}
    if health.CheckedAt.Location()!=time.UTC { t.Fatal("expected UTC timestamp") }
}
