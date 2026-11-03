package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Proxy struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Protocol string `json:"protocol"`
	Host string `json:"host"`
	Port int `json:"port"`
	Username string `json:"username"`
	HasPassword bool `json:"has_password"`
	IsDefault bool `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastCheckStatus string `json:"last_check_status,omitempty"`
	LastCheckLatencyMS int64 `json:"last_check_latency_ms,omitempty"`
	LastCheckAt *time.Time `json:"last_check_at,omitempty"`
}

type ProxyInput struct {
	Name string `json:"name"`
	Protocol string `json:"protocol"`
	Host string `json:"host"`
	Port int `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	ClearPassword bool `json:"clear_password,omitempty"`
}

func validateProxyInput(input ProxyInput) (ProxyInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	if input.ClearPassword && input.Password != "" { return input, errors.New("password and clear_password cannot be used together") }
	if input.Name == "" || len(input.Name) > 80 || len(input.Host) > 253 || input.Host == "" || input.Port < 1 || input.Port > 65535 || len(input.Username) > 128 || len(input.Password) > 256 {
		return input, errors.New("invalid proxy fields")
	}
	switch input.Protocol {
	case "socks5", "socks4", "http", "https":
	default:
		return input, errors.New("proxy protocol must be socks5, socks4, http or https")
	}
	if !validProxyHost(input.Host) {
		return input, errors.New("proxy host must be a valid hostname or IP address")
	}
	return input, nil
}

func validProxyHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	name := strings.TrimSuffix(host, ".")
	if name == "" {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func (s *Service) CreateProxy(ctx context.Context, tenantID string, input ProxyInput) (Proxy, error) {
	input, err := validateProxyInput(input)
	if err != nil { return Proxy{}, err }
	var passwordCipher []byte
	if input.Password != "" {
		passwordCipher, err = encrypt([]byte(input.Password))
		if err != nil { return Proxy{}, err }
	}
	var result Proxy
	err = withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `INSERT INTO tenant_proxies(tenant_id,name,protocol,host,port,username,password_ciphertext)
			VALUES($1,$2,$3,$4,$5,$6,$7)
			RETURNING id::text,name,protocol,host,port,username,password_ciphertext IS NOT NULL,is_default,created_at,updated_at`,
			tenantID,input.Name,input.Protocol,input.Host,input.Port,input.Username,nullBytes(passwordCipher)).
			Scan(&result.ID,&result.Name,&result.Protocol,&result.Host,&result.Port,&result.Username,&result.HasPassword,&result.IsDefault,&result.CreatedAt,&result.UpdatedAt)
	})
	if err != nil { return Proxy{}, fmt.Errorf("create proxy: %w", err) }
	return result,nil
}

func (s *Service) ListProxies(ctx context.Context, tenantID string) ([]Proxy,error) {
	out:=[]Proxy{}
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,`SELECT p.id::text,p.name,p.protocol,p.host,p.port,p.username,p.password_ciphertext IS NOT NULL,p.is_default,p.created_at,p.updated_at,
			COALESCE(c.status,''),COALESCE(c.latency_ms,0),c.checked_at
			FROM tenant_proxies p
			LEFT JOIN LATERAL (
				SELECT status,latency_ms,checked_at FROM tenant_proxy_checks
				WHERE tenant_id=p.tenant_id AND proxy_id=p.id
				ORDER BY checked_at DESC,id DESC LIMIT 1
			) c ON TRUE
			WHERE p.tenant_id=$1 ORDER BY p.created_at DESC`,tenantID)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var p Proxy;var checkedAt sql.NullTime;if err:=rows.Scan(&p.ID,&p.Name,&p.Protocol,&p.Host,&p.Port,&p.Username,&p.HasPassword,&p.IsDefault,&p.CreatedAt,&p.UpdatedAt,&p.LastCheckStatus,&p.LastCheckLatencyMS,&checkedAt);err!=nil{return err};if checkedAt.Valid{value:=checkedAt.Time;p.LastCheckAt=&value};out=append(out,p)}
		return rows.Err()
	})
	return out,err
}

func (s *Service) DeleteProxy(ctx context.Context,tenantID,proxyID string)error{
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var bound bool
		if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND proxy_id=$2)`,tenantID,proxyID).Scan(&bound);err!=nil{return err}
		if bound{return errors.New("proxy is assigned to one or more accounts")}
		res,err:=tx.ExecContext(ctx,`DELETE FROM tenant_proxies WHERE tenant_id=$1 AND id=$2`,tenantID,proxyID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("proxy not found")};return nil
	})
}

func (s *Service) BindProxy(ctx context.Context,tenantID,accountID,proxyID string)error{
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		if proxyID!=""{
			var exists bool
			if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenant_proxies WHERE tenant_id=$1 AND id=$2)`,tenantID,proxyID).Scan(&exists);err!=nil{return err}
			if !exists{return errors.New("proxy not found")}
		}
		res,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET proxy_id=NULLIF($3,'')::uuid,proxy_ciphertext=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`,tenantID,accountID,proxyID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil
	})
}

// CheckProxy performs a bounded TCP reachability check. It deliberately does
// not claim that proxy authentication or Telegram traffic has been verified.
func CheckProxy(ctx context.Context, host string, port int) error {
	if strings.TrimSpace(host)==""||port<1||port>65535{return errors.New("invalid proxy endpoint")}
	address:=net.JoinHostPort(host,strconv.Itoa(port))
	dialer:=net.Dialer{Timeout:4*time.Second}
	conn,err:=dialer.DialContext(ctx,"tcp",address)
	if err!=nil{return fmt.Errorf("proxy TCP connection failed: %w",err)}
	return conn.Close()
}

func (s *Service) CheckProxy(ctx context.Context,tenantID,proxyID string)error{
	if !validAccountUUID(proxyID){return errors.New("proxy not found")}
	var protocol,host,username string
	var port int
	var passwordCipher []byte
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		return tx.QueryRowContext(ctx,`SELECT protocol,host,port,username,password_ciphertext FROM tenant_proxies WHERE tenant_id=$1 AND id=$2`,tenantID,proxyID).Scan(&protocol,&host,&port,&username,&passwordCipher)
	})
	if errors.Is(err,sql.ErrNoRows){return errors.New("proxy not found")};if err!=nil{return err}
	password:=""
	if len(passwordCipher)>0 { plain,err:=decrypt(passwordCipher);if err!=nil{return fmt.Errorf("decrypt proxy credentials: %w",err)};password=string(plain) }
	started:=time.Now()
checkErr:=checkProxyTunnel(ctx,protocol,host,port,username,password,"149.154.167.51:443")
latency:=time.Since(started).Milliseconds();if latency<0{latency=0}
status:="success";message:="proxy authentication and Telegram data-center TCP tunnel succeeded"
if checkErr!=nil{status="failed";message=checkErr.Error();if len(message)>500{message=message[:500]}}
recordCtx,cancelRecord:=context.WithTimeout(context.WithoutCancel(ctx),2*time.Second);defer cancelRecord()
	logErr:=withTenantTx(recordCtx,s.db,tenantID,func(tx *sql.Tx)error{
	_,err:=tx.ExecContext(ctx,`INSERT INTO tenant_proxy_checks(tenant_id,proxy_id,check_type,status,latency_ms,message) VALUES($1,$2,'telegram_dc_tunnel',$3,$4,$5)`,tenantID,proxyID,status,latency,message)
	return err
})
if logErr!=nil { if checkErr!=nil{return fmt.Errorf("%v; could not save proxy check history: %w",checkErr,logErr)};return fmt.Errorf("could not save proxy check history: %w",logErr) }
return checkErr
}

func (s *Service) LoadProxyJSON(ctx context.Context,tx *sql.Tx,tenantID,proxyID string)(string,error){
	var protocol,host,username string;var port int;var passwordCipher []byte
	err:=tx.QueryRowContext(ctx,`SELECT protocol,host,port,username,password_ciphertext FROM tenant_proxies WHERE tenant_id=$1 AND id=$2`,tenantID,proxyID).Scan(&protocol,&host,&port,&username,&passwordCipher)
	if err!=nil{return "",err}
	password:=""
	if len(passwordCipher)>0{plain,err:=decrypt(passwordCipher);if err!=nil{return "",err};password=string(plain)}
	data,err:=json.Marshal(map[string]any{"protocol":protocol,"addr":net.JoinHostPort(host,strconv.Itoa(port)),"username":username,"password":password})
	return string(data),err
}

func (s *Service) UpdateProxy(ctx context.Context,tenantID,proxyID string,input ProxyInput)(Proxy,error){
	input,err:=validateProxyInput(input)
	if err!=nil{return Proxy{},err}
	var passwordCipher []byte
	if input.Password!=""{
		passwordCipher,err=encrypt([]byte(input.Password))
		if err!=nil{return Proxy{},err}
	}
	var result Proxy
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		return tx.QueryRowContext(ctx,`UPDATE tenant_proxies SET name=$3,protocol=$4,host=$5,port=$6,username=$7,
			password_ciphertext=CASE WHEN $8 THEN NULL WHEN $9 THEN $10 ELSE password_ciphertext END,updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
			RETURNING id::text,name,protocol,host,port,username,password_ciphertext IS NOT NULL,created_at,updated_at`,
			tenantID,proxyID,input.Name,input.Protocol,input.Host,input.Port,input.Username,input.ClearPassword,input.Password!="",nullBytes(passwordCipher)).
			Scan(&result.ID,&result.Name,&result.Protocol,&result.Host,&result.Port,&result.Username,&result.HasPassword,&result.CreatedAt,&result.UpdatedAt)
	})
	if err!=nil{return Proxy{},err}
	return result,nil
}


func (s *Service) SetDefaultProxy(ctx context.Context,tenantID,proxyID string)error{
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		if proxyID!="" {
			res,err:=tx.ExecContext(ctx,`UPDATE tenant_proxies SET is_default=FALSE,updated_at=NOW() WHERE tenant_id=$1 AND is_default=TRUE`,tenantID)
			if err!=nil{return err};_ = res
			res,err=tx.ExecContext(ctx,`UPDATE tenant_proxies SET is_default=TRUE,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`,tenantID,proxyID)
			if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("proxy not found")}
		}else{
			if _,err:=tx.ExecContext(ctx,`UPDATE tenant_proxies SET is_default=FALSE,updated_at=NOW() WHERE tenant_id=$1 AND is_default=TRUE`,tenantID);err!=nil{return err}
		}
		return nil
	})
}


// BindProxyBatch atomically binds or clears a tenant proxy for up to 100 accounts.
func (s *Service) BindProxyBatch(ctx context.Context, tenantID string, ids []string, proxyID string) (int,error) {
	ids,_,err:=normalizeBulkGroupInput(ids,"")
	if err!=nil{return 0,err}
	proxyID=strings.TrimSpace(proxyID)
	if len(proxyID)>0 && (len(proxyID)!=36 || proxyID[8]!='-' || proxyID[13]!='-' || proxyID[18]!='-' || proxyID[23]!='-') {return 0,errors.New("invalid proxy id")}
	updated:=0
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		if proxyID!="" {
			var exists bool
			if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenant_proxies WHERE tenant_id=$1 AND id=$2::uuid)`,tenantID,proxyID).Scan(&exists);err!=nil{return err}
			if !exists{return errors.New("proxy not found")}
		}
		for _,id:=range ids {
			res,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET proxy_id=NULLIF($3,'')::uuid,proxy_ciphertext=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid`,tenantID,id,proxyID)
			if err!=nil{return err}
			n,err:=res.RowsAffected();if err!=nil{return err}
			if n!=1{return errors.New("one or more accounts were not found in this tenant")}
			updated+=int(n)
		}
		return nil
	})
	if err!=nil{return 0,err}
	return updated,nil
}

type ProxyCheck struct {
	ID string `json:"id"`
	CheckType string `json:"check_type"`
	Status string `json:"status"`
	LatencyMS int64 `json:"latency_ms"`
	Message string `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

func (s *Service) ListProxyChecks(ctx context.Context,tenantID,proxyID string,limit int)([]ProxyCheck,error) {
	if !validAccountUUID(proxyID){return nil,errors.New("proxy not found")}
	if limit<1 || limit>100 { limit=50 }
	out:=[]ProxyCheck{}
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var exists bool
		if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM tenant_proxies WHERE tenant_id=$1 AND id=$2)",tenantID,proxyID).Scan(&exists);err!=nil{return err}
		if !exists{return errors.New("proxy not found")}
		rows,err:=tx.QueryContext(ctx,"SELECT id::text,check_type,status,latency_ms,message,checked_at FROM tenant_proxy_checks WHERE tenant_id=$1 AND proxy_id=$2 ORDER BY checked_at DESC,id DESC LIMIT $3",tenantID,proxyID,limit)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item ProxyCheck;if err:=rows.Scan(&item.ID,&item.CheckType,&item.Status,&item.LatencyMS,&item.Message,&item.CheckedAt);err!=nil{return err};out=append(out,item)}
		return rows.Err()
	})
	return out,err
}