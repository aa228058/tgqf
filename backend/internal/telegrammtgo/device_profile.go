package telegrammtgo

import (
 "errors"
 "strings"

 device "github.com/mtgo-labs/device-manager"
 "github.com/mtgo-labs/mtgo/telegram"
)

// DeviceProfile creates a stable iOS identity for one Telegram account.
// device-manager derives the model and OS version deterministically from accountID.
func DeviceProfile(accountID, langCode, systemLangCode, langPack, appVersion string) (device.Profile, error) {
 accountID = strings.TrimSpace(accountID)
 if accountID == "" { return device.Profile{}, errors.New("account ID is required for a stable device profile") }
 p := device.IOS.Generate(accountID)
 if langCode != "" { p.LangCode = langCode }
 if systemLangCode != "" { p.SystemLangCode = systemLangCode }
 if langPack != "" { p.LangPack = langPack }
 if appVersion != "" { p.AppVersion = appVersion }
 if p.DeviceModel == "" || p.SystemVersion == "" || p.AppVersion == "" { return device.Profile{}, errors.New("device-manager returned an incomplete profile") }
 return p, nil
}

// ApplyDeviceProfile maps a persisted device-manager profile into mtgo's config.
func ApplyDeviceProfile(cfg *telegram.Config, p device.Profile) {
 p.Apply(cfg)
}
