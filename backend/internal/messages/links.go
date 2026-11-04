package messages

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type DiscoveredLink struct {
	Type string
	PublicURL string
	InviteHash []byte
	InviteCiphertext []byte
}

var telegramLinkPattern = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_./@-])(?:https?://)?(?:www\.)?(?:t\.me|telegram\.me)/(\+[A-Za-z0-9_-]{8,128}|joinchat/[A-Za-z0-9_-]{8,128}|[A-Za-z0-9_]{5,32})(?:[/?#][^\s<>"']*)?`)

var reservedTelegramPaths = map[string]bool{
	"s":true,"addstickers":true,"addemoji":true,"addtheme":true,"addlist":true,
	"share":true,"proxy":true,"socks":true,"c":true,"iv":true,
	"login":true,"confirmphone":true,"setlanguage":true,"bg":true,
}

func ExtractTelegramLinks(text string) []DiscoveredLink {
	if len(text)>32768 { text=text[:32768] }
	matches:=telegramLinkPattern.FindAllStringSubmatch(text,-1)
	out:=make([]DiscoveredLink,0,len(matches))
	seen:=map[string]bool{}
	for _,m:=range matches {
		if len(m)<2 { continue }
		raw:=strings.TrimSpace(m[1])
		if raw=="" { continue }
		if strings.HasPrefix(raw,"+") || strings.HasPrefix(strings.ToLower(raw),"joinchat/") {
			code:=strings.TrimPrefix(raw,"+")
			if strings.HasPrefix(strings.ToLower(code),"joinchat/") { code=code[len("joinchat/"):] }
			if len(code)<8 { continue }
			key:="invite:"+code
			if seen[key] { continue };seen[key]=true
			ciphertext,err:=encryptInvite([]byte(code));if err!=nil { continue }
			hash:=sha256.Sum256([]byte(code))
			out=append(out,DiscoveredLink{Type:"invite",InviteHash:hash[:],InviteCiphertext:ciphertext})
			continue
		}
		username:=strings.Trim(raw,"/")
		if strings.Contains(username,"/") { continue }
		username=strings.TrimPrefix(username,"@")
		if reservedTelegramPaths[strings.ToLower(username)] { continue }
		key:="public:"+strings.ToLower(username)
		if seen[key] { continue };seen[key]=true
		out=append(out,DiscoveredLink{Type:"public",PublicURL:"https://t.me/"+username})
	}
	return out
}

func encryptInvite(plain []byte) ([]byte,error) {
	raw:=strings.TrimSpace(os.Getenv("SESSION_ENCRYPTION_KEY"))
	var key []byte
	if decoded,err:=base64.StdEncoding.DecodeString(raw);err==nil&&len(decoded)==32 { key=decoded
	} else if decoded,err:=base64.RawStdEncoding.DecodeString(raw);err==nil&&len(decoded)==32 { key=decoded
	} else if decoded,err:=hex.DecodeString(raw);err==nil&&len(decoded)==32 { key=decoded
	} else { return nil,errors.New("SESSION_ENCRYPTION_KEY must be a 32-byte key encoded as base64 or hex") }
	block,err:=aes.NewCipher(key);if err!=nil{return nil,err}
	gcm,err:=cipher.NewGCM(block);if err!=nil{return nil,err}
	nonce:=make([]byte,gcm.NonceSize());if _,err=rand.Read(nonce);err!=nil{return nil,err}
	return gcm.Seal(nonce,nonce,plain,nil),nil
}

func normalizePublicTelegramURL(raw string) string {
	u,err:=url.Parse(raw);if err!=nil{return ""}
	if u.Host!="" && !strings.EqualFold(u.Host,"t.me") && !strings.EqualFold(u.Host,"telegram.me") { return "" }
	return strings.TrimSpace(raw)
}
