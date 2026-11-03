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
