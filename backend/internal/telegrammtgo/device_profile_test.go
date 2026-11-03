package telegrammtgo

import (
    "testing"

    device "github.com/mtgo-labs/device-manager"
)

func TestSelectDeviceProfileRequiresExactlyOnePersistedProfile(t *testing.T) {
    profile := device.Profile{DeviceModel:"iPhone",SystemVersion:"18.0",AppVersion:"11.0"}
    if _, err := selectDeviceProfile(nil); err == nil {
        t.Fatal("expected missing persisted profile to fail")
    }
    if _, err := selectDeviceProfile([]device.Profile{profile,profile}); err == nil {
        t.Fatal("expected multiple profiles to fail")
    }
    got, err := selectDeviceProfile([]device.Profile{profile})
    if err != nil { t.Fatalf("selectDeviceProfile() error = %v", err) }
    if got.DeviceModel != profile.DeviceModel || got.SystemVersion != profile.SystemVersion || got.AppVersion != profile.AppVersion {
        t.Fatalf("selected profile = %+v, want %+v", got, profile)
    }
}

func TestSelectDeviceProfileRejectsIncompletePersistedProfile(t *testing.T) {
    _, err := selectDeviceProfile([]device.Profile{{DeviceModel:"iPhone",AppVersion:"11.0"}})
    if err == nil { t.Fatal("expected incomplete persisted profile to fail") }
}
