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
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProxyInput struct {
	Name string `json:"name"`
	Protocol string `json:"protocol"`
	Host string `json:"host"`
	Port int `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func validateProxyInput(input ProxyInput) (ProxyInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	if input.Name == "" || len(input.Name) > 80 || len(input.Host) > 253 || input.Host == "" || input.Port < 1 || input.Port > 65535 || len(input.Username) > 128 || len(input.Password) > 256 {
		return input, errors.New("invalid proxy fields")
	}
	switch input.Protocol {
	case "socks5", "socks4", "http", "https":
	default:
		return input, errors.New("proxy protocol must be socks5, socks4, http or https")
	}
	if strings.ContainsAny(input.Host, " /\\\t\r\n") {
		return input, errors.New("proxy host must be a hostname or IP address")
	}
	return input, nil
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
			RETURNING id::text,name,protocol,host,port,username,password_ciphertext IS NOT NULL,created_at,updated_at`,
			tenantID,input.Name,input.Protocol,input.Host,input.Port,input.Username,nullBytes(passwordCipher)).
			Scan(&result.ID,&result.Name,&result.Protocol,&result.Host,&result.Port,&result.Username,&result.HasPassword,&result.CreatedAt,&result.UpdatedAt)
	})
	if err != nil { return Proxy{}, fmt.Errorf("create proxy: %w", err) }
	return result,nil
}

func (s *Service) ListProxies(ctx context.Context, tenantID string) ([]Proxy,error) {
	out:=[]Proxy{}
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,`SELECT id::text,name,protocol,host,port,username,password_ciphertext IS NOT NULL,created_at,updated_at FROM tenant_proxies WHERE tenant_id=$1 ORDER BY created_at DESC`,tenantID)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var p Proxy;if err:=rows.Scan(&p.ID,&p.Name,&p.Protocol,&p.Host,&p.Port,&p.Username,&p.HasPassword,&p.CreatedAt,&p.UpdatedAt);err!=nil{return err};out=append(out,p)}
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
	var host string;var port int
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		return tx.QueryRowContext(ctx,`SELECT host,port FROM tenant_proxies WHERE tenant_id=$1 AND id=$2`,tenantID,proxyID).Scan(&host,&port)
	})
	if errors.Is(err,sql.ErrNoRows){return errors.New("proxy not found")};if err!=nil{return err}
	return CheckProxy(ctx,host,port)
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
