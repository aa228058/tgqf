package messages

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestExtractTelegramLinksDeduplicatesAndSkipsReservedPaths(t *testing.T) {
	os.Setenv("SESSION_ENCRYPTION_KEY",base64.StdEncoding.EncodeToString(make([]byte,32)))
	defer os.Unsetenv("SESSION_ENCRYPTION_KEY")
	got:=ExtractTelegramLinks("See https://t.me/PublicGroup and t.me/PublicGroup, invite t.me/+AbCdEf123456 and https://telegram.me/joinchat/QwErTy123456. Ignore t.me/addstickers.")
	if len(got)!=3 { t.Fatalf("got %d links, want 3: %#v",len(got),got) }
	public:=0;invites:=0
	for _,link:=range got {
		switch link.Type {
		case "public":
			public++
			if link.PublicURL!="https://t.me/PublicGroup" { t.Errorf("public URL=%q",link.PublicURL) }
		case "invite":
			invites++
			if len(link.InviteHash)!=32||len(link.InviteCiphertext)==0 { t.Errorf("invite not hashed/encrypted: %#v",link) }
		}
	}
	if public!=1||invites!=2 { t.Fatalf("public=%d invites=%d",public,invites) }
}

func TestExtractTelegramLinksDoesNotStorePrivateInviteWithoutKey(t *testing.T) {
	os.Unsetenv("SESSION_ENCRYPTION_KEY")
	got:=ExtractTelegramLinks("t.me/+AbCdEf123456")
	if len(got)!=0 { t.Fatalf("private invite without encryption key must not be stored: %#v",got) }
}

func TestExtractTelegramLinksPreservesInviteCase(t *testing.T) {
	os.Setenv("SESSION_ENCRYPTION_KEY",base64.StdEncoding.EncodeToString(make([]byte,32)))
	defer os.Unsetenv("SESSION_ENCRYPTION_KEY")
	a:=ExtractTelegramLinks("t.me/+AbCdEf123456")
	b:=ExtractTelegramLinks("t.me/+abcdef123456")
	if len(a)!=1||len(b)!=1 { t.Fatalf("expected one invite each: %d %d",len(a),len(b)) }
	if string(a[0].InviteHash)==string(b[0].InviteHash) { t.Fatal("case-sensitive invite hashes unexpectedly match") }
}

func TestExtractTelegramLinksSkipsPreviewAndReservedRoutes(t *testing.T) {
    got := ExtractTelegramLinks("https://t.me/s/PublicGroup/123 t.me/c/123456/789 t.me/share/url")
    if len(got) != 0 {
        t.Fatalf("reserved Telegram routes must not be treated as public usernames: %#v", got)
    }
}
