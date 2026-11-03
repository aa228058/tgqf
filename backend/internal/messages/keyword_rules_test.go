package messages

import (
    "strings"
    "testing"
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
	if _,ok:=keywordRuleMatches(rule,Incoming{ChatType:"private",Text:"alpha and gamma"});!ok {t.Fatal("AND of matching OR groups should pass")}
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
