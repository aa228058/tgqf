package messages

import (
	"context"
	"testing"
)

func TestNormalizeJoinTaskInputRequiresExplicitAuthorization(t *testing.T) {
    base:=JoinTaskInput{Name:"campaign",TargetGroupID:12,AccountIDs:[]string{"11111111-1111-4111-8111-111111111111"},AuthorizationConfirmed:true}
    if _,err:=normalizeJoinTaskInput(base);err!=nil{t.Fatal(err)}
    base.AuthorizationConfirmed=false
    if _,err:=normalizeJoinTaskInput(base);err==nil{t.Fatal("task without explicit authorization was accepted")}
}
func TestNormalizeJoinTaskInputValidatesTargetsAndAccounts(t *testing.T) {
    base:=JoinTaskInput{Name:"campaign",TargetLinkID:5,AccountIDs:[]string{"11111111-1111-4111-8111-111111111111"},AuthorizationConfirmed:true}
    if _,err:=normalizeJoinTaskInput(base);err!=nil{t.Fatal(err)}
    base.TargetLinkID=0
    if _,err:=normalizeJoinTaskInput(base);err==nil{t.Fatal("task without target accepted")}
    base.TargetGroupID=3;base.AccountIDs=[]string{"not-a-uuid"}
    if _,err:=normalizeJoinTaskInput(base);err==nil{t.Fatal("invalid account id accepted")}
}

func TestNormalizePrivateInviteLink(t *testing.T) {
	cases:=[]struct{input,want string;ok bool}{
		{"https://t.me/+AbCdEfGh_123","AbCdEfGh_123",true},
		{"https://t.me/joinchat/AbCdEfGh-123","AbCdEfGh-123",true},
		{"t.me/+AbCdEfGh","AbCdEfGh",true},
		{"https://evil.example/+AbCdEfGh","",false},
		{"https://t.me/+short","",false},
		{"https://t.me/+abc?start=bad","",false},
	}
	for _,tc:=range cases {got,err:=normalizePrivateInviteLink(tc.input);if (err==nil)!=tc.ok {t.Errorf("%q accepted=%v err=%v",tc.input,err==nil,err);continue};if err==nil&&got!=tc.want{t.Errorf("%q got %q want %q",tc.input,got,tc.want)}}
}

func TestNormalizeJoinTaskInputPrivateInviteRequiresExclusiveTarget(t *testing.T) {
	base:=JoinTaskInput{Name:"private join",PrivateInviteLink:"https://t.me/+AbCdEfGh123",AccountIDs:[]string{"11111111-1111-4111-8111-111111111111"},AuthorizationConfirmed:true}
	got,err:=normalizeJoinTaskInput(base);if err!=nil{t.Fatal(err)}
	if got.PrivateInviteLink!="AbCdEfGh123"{t.Fatalf("unexpected normalized invite hash: %q",got.PrivateInviteLink)}
	base.TargetGroupID=12
	if _,err:=normalizeJoinTaskInput(base);err==nil{t.Fatal("private invite combined with public target should be rejected")}
}

func TestRetryJoinTaskRejectsInvalidIDBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	if err := service.RetryJoinTask(context.Background(), "tenant", "not-a-uuid"); err == nil {
		t.Fatal("expected invalid task id to be rejected")
	}
}
