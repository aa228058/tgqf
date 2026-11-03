package telegrammtgo

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	gotdbin "github.com/gotd/td/bin"
	gotdcalls "github.com/gotd/td/telegram/calls"
	gotdtg "github.com/gotd/td/tg"
	"github.com/mtgo-labs/mtgo/telegram"
	mtgotg "github.com/mtgo-labs/mtgo/tg"
)

// mtgoGotdInvoker adapts gotd's TL encoder/decoder to an already-authenticated
// MTGo client. Both libraries speak Telegram's TL wire format, so calls can
// share the account's existing MTProto authorization instead of creating a
// second login/session.
type mtgoGotdInvoker struct {
	client *telegram.Client
}

func (i mtgoGotdInvoker) Invoke(ctx context.Context, input gotdbin.Encoder, output gotdbin.Decoder) error {
	if i.client == nil || input == nil || output == nil {
		return errors.New("voice MTProto adapter is not initialized")
	}
	request := &gotdbin.Buffer{}
	if err := input.Encode(request); err != nil {
		return fmt.Errorf("encode gotd Telegram request: %w", err)
	}
	reader := mtgotg.NewReader(request.Buf)
	mtgoRequest, err := mtgotg.ReadTLObject(reader)
	mtgotg.ReleaseReader(reader)
	if err != nil {
		return fmt.Errorf("decode request for MTGo: %w", err)
	}
	result, err := i.client.Invoke(ctx, mtgoRequest)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("MTGo returned an empty Telegram RPC result")
	}
	var response bytes.Buffer
	if err := result.Encode(&response); err != nil {
		return fmt.Errorf("encode MTGo Telegram response: %w", err)
	}
	decoded := &gotdbin.Buffer{Buf: response.Bytes()}
	if err := output.Decode(decoded); err != nil {
		return fmt.Errorf("decode response for gotd: %w", err)
	}
	if len(decoded.Buf) != 0 {
		return fmt.Errorf("gotd response decoder left %d trailing bytes", len(decoded.Buf))
	}
	return nil
}

type voiceCallBridge struct {
	calls *gotdcalls.Client
}

func newVoiceCallBridge(client *telegram.Client) *voiceCallBridge {
	api := gotdtg.NewClient(mtgoGotdInvoker{client: client})
	callClient := gotdcalls.NewClient(api, gotdcalls.Options{})
	bridge := &voiceCallBridge{calls: callClient}
	client.OnUpdateReceived(func(_ *telegram.Client, updates mtgotg.UpdatesClass) {
		bridge.handleUpdates(context.Background(), updates)
	})
	return bridge
}

func (b *voiceCallBridge) handleUpdates(ctx context.Context, updates mtgotg.UpdatesClass) {
	if b == nil || b.calls == nil || updates == nil {
		return
	}
	var batch []mtgotg.UpdateClass
	switch value := updates.(type) {
	case *mtgotg.Updates:
		batch = value.Updates
	case *mtgotg.UpdatesCombined:
		batch = value.Updates
	case *mtgotg.UpdateShort:
		batch = []mtgotg.UpdateClass{value.Update}
	default:
		return
	}
	for _, update := range batch {
		switch value := update.(type) {
		case *mtgotg.UpdatePhoneCall:
			converted := &gotdtg.UpdatePhoneCall{}
			if err := convertMTGoTL(value, converted); err == nil {
				_ = b.calls.Handle(ctx, converted)
			}
		case *mtgotg.UpdatePhoneCallSignalingData:
			converted := &gotdtg.UpdatePhoneCallSignalingData{}
			if err := convertMTGoTL(value, converted); err == nil {
				_ = b.calls.HandleSignalingData(ctx, converted)
			}
		}
	}
}

func convertMTGoTL(source mtgotg.TLObject, target gotdbin.Decoder) error {
	if source == nil || target == nil {
		return errors.New("cannot convert nil Telegram TL object")
	}
	var wire bytes.Buffer
	if err := source.Encode(&wire); err != nil {
		return err
	}
	buffer := &gotdbin.Buffer{Buf: wire.Bytes()}
	if err := target.Decode(buffer); err != nil {
		return err
	}
	if len(buffer.Buf) != 0 {
		return fmt.Errorf("TL conversion left %d trailing bytes", len(buffer.Buf))
	}
	return nil
}
