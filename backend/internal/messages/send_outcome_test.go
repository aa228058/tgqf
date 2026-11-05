package messages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

type simulatedNetworkTimeout struct{}

func (simulatedNetworkTimeout) Error() string { return "network operation timed out" }
func (simulatedNetworkTimeout) Timeout() bool { return true }
func (simulatedNetworkTimeout) Temporary() bool { return true }

func TestIsAmbiguousTelegramSendError(t *testing.T) {
	cases := []struct {
		name string
		err error
		want bool
	}{
		{"nil", nil, false},
		{"server rejected request", errors.New("USERNAME_INVALID"), false},
		{"FloodWait response", errors.New("FLOOD_WAIT_30"), false},
		{"deadline exceeded", fmt.Errorf("send Telegram message: %w", context.DeadlineExceeded), true},
		{"cancelled during shutdown", context.Canceled, true},
		{"connection EOF", io.EOF, true},
		{"network timeout", simulatedNetworkTimeout{}, true},
		{"connection reset", errors.New("write tcp: connection reset by peer"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAmbiguousTelegramSendError(tc.err); got != tc.want {
				t.Fatalf("isAmbiguousTelegramSendError(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}


func TestClassifyTelegramCapabilityIssue(t *testing.T) {
	tests:=[]struct{name,err,chatType,capability,status string;chatID int64;flood bool}{
		{"peer flood is account level private message","PEER_FLOOD","", "private_message","restricted",0,false},
		{"group write restriction is chat scoped","CHAT_WRITE_FORBIDDEN","group","group_message","restricted",123,false},
		{"media restriction is chat scoped","CHAT_SEND_MEDIA_FORBIDDEN","group","media_send","restricted",123,false},
		{"private target rejection is not global","CHAT_WRITE_FORBIDDEN","private","message_send","restricted",0,false},
		{"flood wait is cooldown","FLOOD_WAIT_30","group","group_message","cooldown",123,true},
		{"generic network error is not a restriction","connection reset","group","", "",0,false},
	}
	for _,tc:=range tests { t.Run(tc.name,func(t *testing.T){
		got:=classifyTelegramCapabilityIssue(errors.New(tc.err),tc.flood,tc.chatType,tc.chatID)
		if tc.capability=="" { if got!=nil { t.Fatalf("unexpected issue: %#v",got) }; return }
		if got==nil { t.Fatal("expected capability issue") }
		if got.capability!=tc.capability || got.status!=tc.status || got.chatID!=tc.chatID { t.Fatalf("got %#v",got) }
	})}
}
