package messages

import (
	"errors"
	"strings"
	"testing"
)

func TestShouldCatalogChat(t *testing.T) {
	for _, tt := range []struct{ chatType string; want bool }{
		{"private", true}, {"group", true}, {"channel", true},
		{"bot", false}, {"unknown", false}, {"", false},
	} {
		if got := ShouldCatalogChat(tt.chatType); got != tt.want {
			t.Errorf("ShouldCatalogChat(%q) = %v, want %v", tt.chatType, got, tt.want)
		}
	}
}

func TestShouldCollect(t *testing.T) {
	tests:=[]struct{name,chatType string;outgoing,replyToOwn,want bool}{
		{"incoming private","private",false,false,true},
		{"outgoing private","private",true,false,false},
		{"incoming group without reply","group",false,false,false},
		{"incoming group reply to own","group",false,true,true},
		{"outgoing group reply","group",true,true,false},
		{"broadcast channel","channel",false,true,false},
		{"bot peer","bot",false,false,false},
		{"unknown peer","unknown",false,true,false},
	}
	for _,tt:=range tests{t.Run(tt.name,func(t *testing.T){if got:=ShouldCollect(tt.chatType,tt.outgoing,tt.replyToOwn);got!=tt.want{t.Fatalf("ShouldCollect()=%v want %v",got,tt.want)}})}
}

func TestValidUUID(t *testing.T) {
	if !validUUID("8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234"){t.Fatal("valid UUID rejected")}
	for _,value:=range []string{"","not-a-uuid","8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234x","8f14e45f_ea3e_4e3d_9c2d_5a9f6b8a1234"}{
		if validUUID(value){t.Errorf("invalid UUID accepted: %q",value)}
	}
}

func TestNormalizeGroupLibraryFilters(t *testing.T) {
	valid, query, err := normalizeGroupLibraryFilters(" CHANNEL ", "  design  ")
	if err != nil { t.Fatalf("valid filters rejected: %v", err) }
	if valid != "channel" || query != "design" { t.Fatalf("unexpected normalized filters: %q %q", valid, query) }

	for _, input := range []struct{ chatType, query string }{
		{chatType:"private"},
		{chatType:"group", query:strings.Repeat("x", 101)},
	} {
		if _, _, err := normalizeGroupLibraryFilters(input.chatType, input.query); !errors.Is(err, ErrInvalidGroupLibraryQuery) {
			t.Errorf("invalid filters (%q, %d chars) error = %v", input.chatType, len(input.query), err)
		}
	}
}
