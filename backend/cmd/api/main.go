package main

import (
 "context"
 "errors"
 "log/slog"
 "net/http"
 "os"
 "os/signal"
 "syscall"
 "time"

 "github.com/aa228058/tgqf/backend/internal/auth"
 "github.com/aa228058/tgqf/backend/internal/config"
 "github.com/aa228058/tgqf/backend/internal/httpapi"
 "github.com/aa228058/tgqf/backend/internal/storage"
)
func main(){
 logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil));slog.SetDefault(logger)
 cfg,err:=config.Load();if err!=nil{logger.Error("load configuration","error",err);os.Exit(1)}
 rootCtx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM);defer stop()
 var store *storage.Store;var authService *auth.Service
 if cfg.DatabaseURL!=""{store,err=storage.Open(rootCtx,cfg.DatabaseURL,cfg.RedisURL);if err!=nil{logger.Error("initialize storage","error",err);os.Exit(1)};defer store.Close();authService=auth.New(store.DB)
  if cfg.SuperAdminUsername!=""{if err=authService.EnsureSuperAdmin(rootCtx,cfg.SuperAdminUsername,cfg.SuperAdminPassword);err!=nil{logger.Error("initialize super admin","error",err);os.Exit(1)}}
 }else{logger.Warn("database and Redis are not configured; readiness and authentication are unavailable")}
 server:=&http.Server{Addr:cfg.HTTPAddr,Handler:httpapi.NewRouter(store,authService),ReadHeaderTimeout:5*time.Second,IdleTimeout:60*time.Second}
 serverErrors:=make(chan error,1);go func(){logger.Info("API server starting","address",cfg.HTTPAddr,"service","TG账号管理系统");serverErrors<-server.ListenAndServe()}()
 select{case err:=<-serverErrors:if err!=nil&&!errors.Is(err,http.ErrServerClosed){logger.Error("API server stopped unexpectedly","error",err);os.Exit(1)};case<-rootCtx.Done():ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();if err:=server.Shutdown(ctx);err!=nil{logger.Error("graceful shutdown failed","error",err);_=server.Close()}}
}
