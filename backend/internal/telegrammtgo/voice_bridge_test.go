package telegrammtgo

import (
	"bytes"
	"testing"

	gotdbin "github.com/gotd/td/bin"
	gotdtg "github.com/gotd/td/tg"
	mtgotg "github.com/mtgo-labs/mtgo/tg"
)

func TestVoiceBridgeTLRequestCompatibility(t *testing.T) {
	request:=&gotdtg.PhoneGetCallConfigRequest{}
	encoded:=&gotdbin.Buffer{}
	if err:=request.Encode(encoded);err!=nil{t.Fatal(err)}
	reader:=mtgotg.NewReader(encoded.Buf)
	decoded,err:=mtgotg.ReadTLObject(reader)
	mtgotg.ReleaseReader(reader)
	if err!=nil{t.Fatal(err)}
	if _,ok:=decoded.(*mtgotg.PhoneGetCallConfigRequest);!ok{t.Fatalf("decoded request type %T",decoded)}
}

func TestVoiceBridgeUpdateSignalingConversion(t *testing.T) {
	source:=&mtgotg.UpdatePhoneCallSignalingData{PhoneCallID:987654321,Data:[]byte{1,2,3,4}}
	target:=&gotdtg.UpdatePhoneCallSignalingData{}
	if err:=convertMTGoTL(source,target);err!=nil{t.Fatal(err)}
	if target.PhoneCallID!=source.PhoneCallID||!bytes.Equal(target.Data,source.Data){t.Fatalf("converted update mismatch: %#v",target)}
}

func TestVoiceBridgeResponseCompatibility(t *testing.T) {
	source:=&mtgotg.DataJSON{Data:"{}"}
	var wire bytes.Buffer
	if err:=source.Encode(&wire);err!=nil{t.Fatal(err)}
	target:=&gotdtg.DataJSON{}
	buffer:=&gotdbin.Buffer{Buf:wire.Bytes()}
	if err:=target.Decode(buffer);err!=nil{t.Fatal(err)}
	if target.Data!="{}"||len(buffer.Buf)!=0{t.Fatalf("decoded response=%#v trailing=%d",target,len(buffer.Buf))}
}
