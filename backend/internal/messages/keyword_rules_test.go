package messages

import (
    "encoding/json"
    "strings"
    "testing"
    "time"
)

func TestNormalizeKeywordRule(t *testing.T) {
    rule,err:=normalizeKeywordRule(KeywordRule{Name:"  lead gen ",MatchMode:"ANY",Keywords:[]string{" hello ","HELLO"},ChatTypes:[]string{"private","group"}})
    if err!=nil{t.Fatal(err)}
    if rule.Name!="lead gen"||len(rule.Keywords)!=1||rule.Keywords[0]!="hello"{t.Fatalf("unexpected normalized rule: %#v",rule)}
    if _,err:=normalizeKeywordRule(KeywordRule{Name:"bad",MatchMode:"regex",Keywords:[]string{"("}});err==nil{t.Fatal("invalid regex accepted")}
    if _,err:=normalizeKeywordRule(KeywordRule{Name:"bad",MatchMode:"all",Keywords:[]string{strings.Repeat("x",257)}});err==nil{t.Fatal("oversized keyword accepted")}
    if _,err:=normalizeKeywordRule(KeywordRule{Name:"bad",MatchMode:"or",Keywords:[]string{"x"}});err==nil{t.Fatal("unsupported mode accepted")}
    if _,err:=normalizeKeywordRule(KeywordRule{Name:"exact",MatchMode:"exact",Keywords:[]string{"x"}});err!=nil{t.Fatalf("exact mode rejected: %v",err)}
}

func TestKeywordRuleMatchesModesAndExclusions(t *testing.T) {
    base:=Incoming{ChatType:"private",Text:"Hello Telegram automation"}
    any:=KeywordRule{MatchMode:"any",Keywords:[]string{"hello","missing"},ChatTypes:[]string{"private"}}
    terms,ok:=keywordRuleMatches(any,base);if !ok||len(terms)!=1||terms[0]!="hello"{t.Fatalf("any match = %v, %v",terms,ok)}
    all:=KeywordRule{MatchMode:"all",Keywords:[]string{"hello","automation"},ChatTypes:[]string{"private"}}
    if _,ok:=keywordRuleMatches(all,base);!ok{t.Fatal("all rule should match")}
    all.Keywords=append(all.Keywords,"absent");if _,ok:=keywordRuleMatches(all,base);ok{t.Fatal("all rule matched without every term")}
    regex:=KeywordRule{MatchMode:"regex",Keywords:[]string{"(?i)telegram\\s+automation"},ChatTypes:[]string{"private"}}
    if _,ok:=keywordRuleMatches(regex,base);!ok{t.Fatal("regex rule should match")}
    any.ExcludeTerms=[]string{"telegram"};if _,ok:=keywordRuleMatches(any,base);ok{t.Fatal("excluded message matched")}
    if _,ok:=keywordRuleMatches(any,Incoming{ChatType:"private",Text:base.Text,IsOutgoing:true});ok{t.Fatal("outgoing message matched")}
    if _,ok:=keywordRuleMatches(any,Incoming{ChatType:"channel",Text:base.Text});ok{t.Fatal("unselected chat type matched")}
}

func TestKeywordRuleExactMatch(t *testing.T) {
	rule := KeywordRule{MatchMode:"exact", Keywords:[]string{"hello world"}, ChatTypes:[]string{"private"}}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"HELLO WORLD"});!ok {
		t.Fatal("exact match should honor default case-insensitive behavior")
	}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"hello world!"});ok {
		t.Fatal("exact match must reject text with additional characters")
	}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"prefix hello world"});ok {
		t.Fatal("exact match must reject partial-message matches")
	}
	rule.CaseSensitive=true
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"HELLO WORLD"});ok {
		t.Fatal("case-sensitive exact match unexpectedly ignored case")
	}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"hello world"});!ok {
		t.Fatal("case-sensitive exact match should match identical text")
	}
}

func TestKeywordRuleCaseSensitivity(t *testing.T) {
    in:=Incoming{ChatType:"group",Text:"HELLO"}
    insensitive:=KeywordRule{MatchMode:"any",Keywords:[]string{"hello"},ChatTypes:[]string{"group"}}
    if _,ok:=keywordRuleMatches(insensitive,in);!ok{t.Fatal("case-insensitive rule should match")}
    sensitive:=insensitive;sensitive.CaseSensitive=true
    if _,ok:=keywordRuleMatches(sensitive,in);ok{t.Fatal("case-sensitive rule unexpectedly matched")}
}

func TestKeywordRuleAccountChatAndMediaFilters(t *testing.T) {
	accountA := "11111111-1111-4111-8111-111111111111"
	accountB := "22222222-2222-4222-8222-222222222222"
	rule, err := normalizeKeywordRule(KeywordRule{
		Name:"filtered", MatchMode:"any", Keywords:[]string{"hello"},
		ChatTypes:[]string{"group"}, AccountIDs:[]string{accountA},
		ChatIDs:[]int64{-100123}, MediaTypes:[]string{"photo"},
	})
	if err != nil { t.Fatal(err) }
	base := Incoming{AccountID:accountA, ChatID:-100123, ChatType:"group", Text:"hello", MediaType:"photo"}
	if _,ok:=keywordRuleMatches(rule,base); !ok { t.Fatal("matching account, chat and media should pass") }
	cases := []struct{name string; in Incoming}{
		{"other account", Incoming{AccountID:accountB,ChatID:-100123,ChatType:"group",Text:"hello",MediaType:"photo"}},
		{"other chat", Incoming{AccountID:accountA,ChatID:-100999,ChatType:"group",Text:"hello",MediaType:"photo"}},
		{"other media", Incoming{AccountID:accountA,ChatID:-100123,ChatType:"group",Text:"hello",MediaType:"video"}},
		{"other chat type", Incoming{AccountID:accountA,ChatID:-100123,ChatType:"private",Text:"hello",MediaType:"photo"}},
	}
	for _,tc := range cases { t.Run(tc.name,func(t *testing.T){if _,ok:=keywordRuleMatches(rule,tc.in);ok{t.Fatal("non-matching filter passed")}}) }
}

func TestNormalizeKeywordRuleRejectsInvalidFilters(t *testing.T) {
	_,err:=normalizeKeywordRule(KeywordRule{Name:"bad account",MatchMode:"any",Keywords:[]string{"x"},AccountIDs:[]string{"not-a-uuid"}})
	if err==nil {t.Fatal("invalid account filter accepted")}
	_,err=normalizeKeywordRule(KeywordRule{Name:"bad chat",MatchMode:"any",Keywords:[]string{"x"},ChatIDs:[]int64{0}})
	if err==nil {t.Fatal("zero chat filter accepted")}
	_,err=normalizeKeywordRule(KeywordRule{Name:"bad media",MatchMode:"any",Keywords:[]string{"x"},MediaTypes:[]string{"executable"}})
	if err==nil {t.Fatal("unsupported media filter accepted")}
}

func TestKeywordRuleMediaOnly(t *testing.T) {
	rule,err:=normalizeKeywordRule(KeywordRule{
		Name:"photo intake",MatchMode:"any",MediaTypes:[]string{"photo"},
		ChatTypes:[]string{"private"},
	})
	if err!=nil {t.Fatalf("media-only rule rejected: %v",err)}
	matched,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",MediaType:"photo"})
	if !ok || len(matched)!=1 || matched[0]!="media:photo" {t.Fatalf("media-only match = %v, %v",matched,ok)}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",MediaType:"video"});ok {t.Fatal("wrong media type matched")}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",MediaType:"photo",IsOutgoing:true});ok {t.Fatal("outgoing media matched")}
}

func TestKeywordRuleRequiresKeywordsOrMediaType(t *testing.T) {
	_,err:=normalizeKeywordRule(KeywordRule{Name:"empty",MatchMode:"any"})
	if err==nil {t.Fatal("rule without keywords or media filter accepted")}
}

func TestKeywordRuleSenderAndChatMetadataFilters(t *testing.T) {
	rule,err:=normalizeKeywordRule(KeywordRule{
		Name:"trusted source",MatchMode:"any",Keywords:[]string{"offer"},
		ChatTypes:[]string{"group"},ChatTitles:[]string{"Sales Room"},
		ChatUsernames:[]string{"salesroom"},SenderIDs:[]int64{42},
		SenderUsernames:[]string{"alice"},SenderNames:[]string{"Alice Smith"},
	})
	if err!=nil {t.Fatal(err)}
	base:=Incoming{ChatType:"group",ChatTitle:"Sales Room",ChatUsername:"salesroom",SenderID:42,SenderUsername:"alice",SenderName:"Alice Smith",Text:"New offer"}
	if _,ok:=keywordRuleMatches(rule,base);!ok {t.Fatal("matching sender and chat metadata should pass")}
	cases:=[]struct{name string; mutate func(*Incoming)}{
		{"wrong title",func(v *Incoming){v.ChatTitle="Other"}},
		{"wrong chat username",func(v *Incoming){v.ChatUsername="other"}},
		{"wrong sender id",func(v *Incoming){v.SenderID=99}},
		{"wrong sender username",func(v *Incoming){v.SenderUsername="bob"}},
		{"wrong sender name",func(v *Incoming){v.SenderName="Bob Jones"}},
	}
	for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){in:=base;tc.mutate(&in);if _,ok:=keywordRuleMatches(rule,in);ok{t.Fatal("metadata mismatch passed")}})}
}
func TestNormalizeKeywordRuleRejectsInvalidSenderFilter(t *testing.T) {
	_,err:=normalizeKeywordRule(KeywordRule{Name:"bad sender",MatchMode:"any",Keywords:[]string{"x"},SenderIDs:[]int64{0}})
	if err==nil {t.Fatal("zero sender id accepted")}
}

func TestNormalizeKeywordRuleReplyAction(t *testing.T) {
	rule,err:=normalizeKeywordRule(KeywordRule{Name:"reply",MatchMode:"any",Keywords:[]string{"hello"},ReplyText:"  Welcome!  "})
	if err!=nil {t.Fatalf("valid reply action rejected: %v",err)}
	if rule.ReplyText!="Welcome!" {t.Fatalf("reply text not normalized: %q",rule.ReplyText)}
	_,err=normalizeKeywordRule(KeywordRule{Name:"too long",MatchMode:"any",Keywords:[]string{"hello"},ReplyText:strings.Repeat("x",4097)})
	if err==nil {t.Fatal("reply action longer than Telegram message limit accepted")}
}

func TestKeywordRuleGroupedORAndANDExpressions(t *testing.T) {
	rule,err:=normalizeKeywordRule(KeywordRule{
		Name:"grouped",MatchMode:"any",GroupOperator:"all",
		KeywordGroups:[][]string{{"alpha","beta"},{"gamma"}},
	})
	if err!=nil {t.Fatalf("grouped rule rejected: %v",err)}
	if matched,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"alpha and gamma"});!ok {t.Fatal("AND of matching OR groups should pass")} else if len(matched)!=2||matched[0]!="group:alpha"||matched[1]!="group:gamma" {t.Fatalf("unexpected grouped match details: %v",matched)}
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"alpha only"});ok {t.Fatal("AND group should fail when one group is absent")}
	rule.GroupOperator="any"
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"beta only"});!ok {t.Fatal("OR between groups should pass when one group matches")}
}

func TestNormalizeKeywordRuleRejectsInvalidGroups(t *testing.T) {
	_,err:=normalizeKeywordRule(KeywordRule{Name:"empty group",MatchMode:"any",KeywordGroups:[][]string{{}}})
	if err==nil {t.Fatal("empty keyword group accepted")}
	_,err=normalizeKeywordRule(KeywordRule{Name:"bad operator",MatchMode:"any",Keywords:[]string{"x"},GroupOperator:"xor"})
	if err==nil {t.Fatal("unsupported group operator accepted")}
}

func TestNormalizeKeywordRuleChatSpecificReplies(t *testing.T) {
	rule,err:=normalizeKeywordRule(KeywordRule{
		Name:"separate actions",MatchMode:"any",Keywords:[]string{"hello"},
		ReplyText:"general",
		ReplyTextByChatType:map[string]string{"PRIVATE":"private reply","group":"group reply"},
	})
	if err!=nil {t.Fatalf("chat-specific replies rejected: %v",err)}
	if got:=keywordReplyTextForChat(rule,"private");got!="private reply" {t.Fatalf("private action = %q",got)}
	if got:=keywordReplyTextForChat(rule,"supergroup");got!="group reply" {t.Fatalf("group action = %q",got)}
	if got:=keywordReplyTextForChat(rule,"channel");got!="" {t.Fatalf("channel action should be disabled, got %q",got)}
	if got:=keywordReplyTextForChat(rule,"private_user");got!="general" {t.Fatalf("fallback action = %q",got)}
}

func TestNormalizeKeywordRuleRejectsInvalidChatSpecificReplies(t *testing.T) {
	_,err:=normalizeKeywordRule(KeywordRule{Name:"bad key",MatchMode:"any",Keywords:[]string{"x"},ReplyTextByChatType:map[string]string{"channel":"not allowed"}})
	if err==nil {t.Fatal("unsupported chat-specific action key accepted")}
	_,err=normalizeKeywordRule(KeywordRule{Name:"too long",MatchMode:"any",Keywords:[]string{"x"},ReplyTextByChatType:map[string]string{"private":strings.Repeat("x",4097)}})
	if err==nil {t.Fatal("oversized chat-specific reply accepted")}
}

func TestKeywordActionRetryDelay(t *testing.T) {
    tests:=[]struct{name string;attempt int;flood time.Duration;want time.Duration}{
        {"first retry",1,0,30*time.Second},
        {"second retry",2,0,60*time.Second},
        {"bounded retry",100,0,15*time.Minute},
        {"flood wait exact plus safety",1,90*time.Second,91*time.Second},
        {"long flood wait is not capped",2,2*time.Hour,2*time.Hour+time.Second},
        {"zero attempt minimum",0,0,30*time.Second},
    }
    for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){
        if got:=keywordActionRetryDelay(tt.attempt,tt.flood);got!=tt.want {t.Fatalf("delay=%s want %s",got,tt.want)}
    }) }
}

func TestNormalizeKeywordRuleContactAction(t *testing.T) {
    valid:=KeywordRule{
        Name:"share contact",MatchMode:"any",Keywords:[]string{"contact"},
        ChatTypes:[]string{"private","group"},ActionType:"contact",
        ContactPhone:"+15551234567",ContactFirstName:"Sales",ContactLastName:"Team",
    }
    got,err:=normalizeKeywordRule(valid)
    if err!=nil {t.Fatalf("valid contact action rejected: %v",err)}
    if got.ActionType!="contact" || got.ContactPhone!=valid.ContactPhone || got.ContactFirstName!=valid.ContactFirstName {
        t.Fatalf("contact action not preserved: %#v",got)
    }
    invalid:=valid
    invalid.ContactPhone=""
    if _,err:=normalizeKeywordRule(invalid);err==nil {t.Fatal("contact action without phone accepted")}
    invalid=valid
    invalid.ChatTypes=[]string{"private","channel"}
    if _,err:=normalizeKeywordRule(invalid);err==nil {t.Fatal("contact action enabled for channel chats")}
    invalid=valid
    invalid.ReplyText="also send text"
    if _,err:=normalizeKeywordRule(invalid);err==nil {t.Fatal("mixed contact and text action accepted")}
    invalid=valid
    invalid.ActionType="unsupported"
    if _,err:=normalizeKeywordRule(invalid);err==nil {t.Fatal("unsupported action type accepted")}
}


func TestNormalizeKeywordRuleMediaAction(t *testing.T) {
    valid:=KeywordRule{Name:"send brochure",MatchMode:"any",Keywords:[]string{"brochure"},ChatTypes:[]string{"private","group"},ActionType:"media",MediaAssetID:"11111111-1111-4111-8111-111111111111",MediaType:"document",MediaCaption:"Product brochure"}
    got,err:=normalizeKeywordRule(valid);if err!=nil{t.Fatalf("valid media action rejected: %v",err)}
    if got.MediaType!="document"||got.MediaCaption!=valid.MediaCaption{t.Fatalf("media action not preserved: %#v",got)}
    invalid:=valid;invalid.MediaAssetID="not-a-uuid";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("invalid media asset id accepted")}
    invalid=valid;invalid.MediaType="animation";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("unsupported media type accepted")}
    invalid=valid;invalid.ReplyText="also text";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("mixed text/media action accepted")}
    invalid=valid;invalid.MediaCaption=strings.Repeat("x",1025);if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("oversized caption accepted")}
    invalid=valid;invalid.ChatTypes=[]string{"private","channel"};if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("channel media action accepted")}
}


func TestNormalizeKeywordMediaAlbum(t *testing.T) {
    valid:=KeywordRule{Name:"catalog album",MatchMode:"any",Keywords:[]string{"catalog"},ChatTypes:[]string{"private","group"},ActionType:"media_album",MediaAlbumItems:[]KeywordMediaAlbumItem{
        {AssetID:"11111111-1111-4111-8111-111111111111",MediaType:"photo",Caption:"Cover"},
        {AssetID:"22222222-2222-4222-8222-222222222222",MediaType:"video",Caption:"Demo"},
    }}
    got,err:=normalizeKeywordRule(valid);if err!=nil{t.Fatalf("valid album rejected: %v",err)}
    if len(got.MediaAlbumItems)!=2||got.MediaAlbumItems[1].MediaType!="video"{t.Fatalf("album order/payload changed: %#v",got.MediaAlbumItems)}
    invalid:=valid;invalid.MediaAlbumItems=invalid.MediaAlbumItems[:1];if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("one-item album accepted")}
    invalid=valid;invalid.MediaAlbumItems[1].AssetID=invalid.MediaAlbumItems[0].AssetID;if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("duplicate album asset accepted")}
    invalid=valid;invalid.MediaAlbumItems[1].MediaType="audio";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("unsupported album media type accepted")}
    invalid=valid;invalid.MediaAlbumItems[1].MediaType="document";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("mixed document/photo album accepted")}
    invalid=valid;invalid.ReplyText="extra text";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("mixed text/album action accepted")}
    invalid=valid;invalid.ChatTypes=[]string{"private","channel"};if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("channel album action accepted")}
    invalid=valid;invalid.MediaAlbumItems[0].AssetID="bad-id";if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("invalid album asset UUID accepted")}
    invalid=valid;invalid.MediaAlbumItems[0].Caption=strings.Repeat("中",1025);if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("oversized Unicode album caption accepted")}
    invalid=valid;invalid.MediaAlbumItems=append(invalid.MediaAlbumItems,
        KeywordMediaAlbumItem{AssetID:"33333333-3333-4333-8333-333333333333",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"44444444-4444-4444-8444-444444444444",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"55555555-5555-4555-8555-555555555555",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"66666666-6666-4666-8666-666666666666",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"77777777-7777-4777-8777-777777777777",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"88888888-8888-4888-8888-888888888888",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"99999999-9999-4999-8999-999999999999",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",MediaType:"photo"},
        KeywordMediaAlbumItem{AssetID:"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",MediaType:"photo"})
    if _,err:=normalizeKeywordRule(invalid);err==nil{t.Fatal("11-item album accepted")}
}


func TestKeywordAlbumJSONEmptyIsArray(t *testing.T) {
    if got:=keywordAlbumJSON(nil);got!="[]" { t.Fatalf("empty album JSON = %q, want []",got) }
    items:=[]KeywordMediaAlbumItem{{AssetID:"11111111-1111-4111-8111-111111111111",MediaType:"photo"}}
    if got:=keywordAlbumJSON(items);!strings.Contains(got,`"media_type":"photo"`) { t.Fatalf("album JSON missing item fields: %s",got) }
}

func TestKeywordActionRecordSerializesAllTelegramMessageIDs(t *testing.T) {
    record := KeywordActionRecord{
        ID: "job-1",
        ActionType: "media_album",
        Status: "sent",
        TelegramMessageIDs: []int64{101, 102, 103},
    }
    encoded, err := json.Marshal(record)
    if err != nil { t.Fatal(err) }
    var decoded KeywordActionRecord
    if err := json.Unmarshal(encoded, &decoded); err != nil { t.Fatal(err) }
    if len(decoded.TelegramMessageIDs) != 3 || decoded.TelegramMessageIDs[0] != 101 || decoded.TelegramMessageIDs[2] != 103 {
        t.Fatalf("Telegram message IDs were not preserved: %#v", decoded.TelegramMessageIDs)
    }
}
