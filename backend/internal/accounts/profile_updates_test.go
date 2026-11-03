package accounts

import (
	"testing"
	"time"
)

func TestNormalizeProfileUpdate(t *testing.T) {
	valid,err:=normalizeProfileUpdate(ProfileUpdateInput{AccountID:"123e4567-e89b-12d3-a456-426614174000",FirstName:"  Alice  ",About:"  hello  ",GenerateUsername:true})
	if err!=nil{t.Fatal(err)}
	if valid.FirstName!="Alice"||valid.About!="hello"||!valid.GenerateUsername{t.Fatalf("unexpected normalized profile: %#v",valid)}
	cases:=[]ProfileUpdateInput{
		{AccountID:"123e4567-e89b-12d3-a456-426614174000"},
		{AccountID:"bad",FirstName:"Alice"},
		{AccountID:"123e4567-e89b-12d3-a456-426614174000",FirstName:string(make([]byte,0))},
		{AccountID:"123e4567-e89b-12d3-a456-426614174000",About:string(make([]byte,71))},
		{AccountID:"123e4567-e89b-12d3-a456-426614174000"},
	}
	for i,in:=range cases{if _,err:=normalizeProfileUpdate(in);err==nil{t.Errorf("case %d: expected validation error",i)}}
}

func TestProfileUpdateRetrySchedule(t *testing.T) {
	status,delay:=profileUpdateRetry(0)
	if status!="pending"||delay!=30*time.Second{t.Fatalf("first retry = %s %s",status,delay)}
	status,delay=profileUpdateRetry(3)
	if status!="pending"||delay!=4*time.Minute{t.Fatalf("fourth attempt retry = %s %s",status,delay)}
	status,delay=profileUpdateRetry(4)
	if status!="failed"||delay!=0{t.Fatalf("fifth attempt = %s %s",status,delay)}
}
