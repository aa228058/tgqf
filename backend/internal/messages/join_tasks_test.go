package messages

import "testing"

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
