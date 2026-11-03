package messages

import "testing"

func TestNormalizeGroupCorrectionFields(t *testing.T) {
    title:="  A useful public group  "
    username:="@valid_group"
    members:=1200
    verified:=true
    got,raw,err:=normalizeGroupCorrectionFields(GroupCorrectionFields{Title:&title,Username:&username,MemberCount:&members,IsVerified:&verified})
    if err!=nil {t.Fatalf("valid correction rejected: %v",err)}
    if *got.Title!="A useful public group"||*got.Username!="valid_group"||*got.MemberCount!=1200||! *got.IsVerified {t.Fatalf("unexpected normalized fields: %+v",got)}
    if len(raw)==0 {t.Fatal("expected serialized correction fields")}
}

func TestNormalizeGroupCorrectionFieldsRejectsUnsafeOrEmptyValues(t *testing.T) {
    empty:="  "
    badUsername:="https://t.me/not_a_username"
    negative:=-1
    tooLong:=string(make([]byte,129))
    cases:=[]struct{name string;fields GroupCorrectionFields}{
        {"empty",GroupCorrectionFields{}},
        {"blank title",GroupCorrectionFields{Title:&empty}},
        {"invalid username",GroupCorrectionFields{Username:&badUsername}},
        {"negative member count",GroupCorrectionFields{MemberCount:&negative}},
        {"oversized title",GroupCorrectionFields{Title:&tooLong}},
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            if _,_,err:=normalizeGroupCorrectionFields(tc.fields);err==nil {t.Fatal("expected validation error")}
        })
    }
}
