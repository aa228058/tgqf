package main

import (
	"testing"
	"errors"
	"time"
	"strings"

	"github.com/aa228058/tgqf/backend/internal/accounts"
)

func TestReconnectDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want time.Duration
	}{
		{attempt: 0, want: 30 * time.Second},
		{attempt: 1, want: 30 * time.Second},
		{attempt: 2, want: 60 * time.Second},
		{attempt: 3, want: 2 * time.Minute},
		{attempt: 4, want: 4 * time.Minute},
		{attempt: 5, want: 8 * time.Minute},
		{attempt: 6, want: 15 * time.Minute},
		{attempt: 7, want: 15 * time.Minute},
	}
	for _, tt := range tests {
		if got := reconnectDelay(tt.attempt); got != tt.want {
			t.Errorf("reconnectDelay(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}

func TestScheduleReconnect(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	state := reconnectState{}
	for attempt := 1; attempt <= 7; attempt++ {
		delay := scheduleReconnect(&state, now)
		if state.attempts != attempt {
			t.Fatalf("attempt = %d, want %d", state.attempts, attempt)
		}
		if delay != reconnectDelay(attempt) {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, delay, reconnectDelay(attempt))
		}
		if !state.nextTry.Equal(now.Add(delay)) {
			t.Fatalf("attempt %d nextTry = %s, want %s", attempt, state.nextTry, now.Add(delay))
		}
		if now.Before(state.nextTry) {
			// A reconciliation tick before the deadline must skip this account.
			continue
		}
		t.Fatalf("retry deadline was not in the future")
	}
}

func TestGenerateRandomTelegramUsername(t *testing.T) {
	tests:=[]struct{name string;settings accounts.UsernameGenerationSettings;prefix string;length int}{
		{name:"tenant prefix alphanumeric",settings:accounts.UsernameGenerationSettings{Prefix:"brand_",RandomLength:12,Charset:"alphanumeric"},prefix:"brand_",length:18},
		{name:"letters only",settings:accounts.UsernameGenerationSettings{Prefix:"team",RandomLength:8,Charset:"letters"},prefix:"team",length:12},
		{name:"empty prefix",settings:accounts.UsernameGenerationSettings{Prefix:"",RandomLength:10,Charset:"alphanumeric"},length:10},
	}
	for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){
		for i:=0;i<100;i++ {
			username,err:=generateRandomTelegramUsername(tt.settings);if err!=nil{t.Fatal(err)}
			if len(username)!=tt.length{t.Fatalf("length %d, want %d: %q",len(username),tt.length,username)}
			if tt.prefix!=""&&!strings.HasPrefix(username,tt.prefix){t.Fatalf("username %q missing prefix %q",username,tt.prefix)}
			if username[0]<'a'||username[0]>'z'{t.Fatalf("username must start with lowercase letter: %q",username)}
			for _,ch:=range username {
				if !((ch>='a'&&ch<='z')||(ch>='0'&&ch<='9')||ch=='_'){t.Fatalf("invalid character in %q",username)}
				if tt.settings.Charset=="letters"&&!(ch>='a'&&ch<='z')&&ch!='_'{t.Fatalf("non-letter in letters-only username %q",username)}
			}
		}
	})}
}

func TestIsPermanentJoinFailure(t *testing.T) {
    tests := []struct { name string; err error; want bool }{
        {"expired invite", errors.New("INVITE_HASH_EXPIRED"), true},
        {"private channel", errors.New("CHANNEL_PRIVATE"), true},
        {"already participant", errors.New("USER_ALREADY_PARTICIPANT"), false},
		{"approval request", errors.New("INVITE_REQUEST_SENT"), false},
        {"flood wait", errors.New("FLOOD_WAIT_120"), false},
        {"timeout", errors.New("context deadline exceeded"), false},
        {"temporary rpc", errors.New("INTERNAL"), false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := isPermanentJoinFailure(tt.err); got != tt.want {
                t.Fatalf("isPermanentJoinFailure(%v)=%v, want %v", tt.err, got, tt.want)
            }
        })
    }
}
