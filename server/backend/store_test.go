package backend

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openclawdeploy/internal/shared"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return newTestStoreAtPath(t, filepath.Join(t.TempDir(), stateDBFilename))
}

func newTestStoreAtPath(t *testing.T, path string) *Store {
	t.Helper()

	store, err := NewStore(path, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestBindDeviceMatchesLegacyFormattedID(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateUser("user1", "user1@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	legacyID := "likeqi|00:15:5d:a3:46:da|2026-03-11 17:07:15|100.64.0.3"
	normalizedID := shared.NormalizeDeviceID(legacyID)

	if _, err := store.BindDevice(legacyID, user.ID); err != nil {
		t.Fatalf("bind legacy id: %v", err)
	}
	if _, err := store.BindDevice(normalizedID, user.ID); err != nil {
		t.Fatalf("bind normalized id: %v", err)
	}

	boundUser, ok := store.GetUserByID(user.ID)
	if !ok {
		t.Fatal("bound user not found")
	}
	devices := store.ListDevices(boundUser)
	if len(devices) != 1 {
		t.Fatalf("unexpected device count: %d", len(devices))
	}
	if devices[0].DeviceID != normalizedID {
		t.Fatalf("unexpected device id: %s", devices[0].DeviceID)
	}
}

func TestDeleteDeviceAllowsOwner(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateUser("owner1", "owner1@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	owner, ok := store.GetUserByID(user.ID)
	if !ok {
		t.Fatal("owner not found")
	}

	if _, err := store.BindDevice("device-delete-1", owner.ID); err != nil {
		t.Fatalf("bind device: %v", err)
	}
	if err := store.DeleteDevice("device-delete-1", owner); err != nil {
		t.Fatalf("delete device: %v", err)
	}

	devices := store.ListDevices(owner)
	if len(devices) != 0 {
		t.Fatalf("expected no devices after delete, got %d", len(devices))
	}
}

func TestDeleteDeviceRejectsNonOwner(t *testing.T) {
	store := newTestStore(t)

	ownerPublic, err := store.CreateUser("owner2", "owner2@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	viewerPublic, err := store.CreateUser("viewer2", "viewer2@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	owner, ok := store.GetUserByID(ownerPublic.ID)
	if !ok {
		t.Fatal("owner not found")
	}
	viewer, ok := store.GetUserByID(viewerPublic.ID)
	if !ok {
		t.Fatal("viewer not found")
	}

	if _, err := store.BindDevice("device-delete-2", owner.ID); err != nil {
		t.Fatalf("bind device: %v", err)
	}
	if err := store.DeleteDevice("device-delete-2", viewer); err == nil || err.Error() != "permission denied" {
		t.Fatalf("unexpected delete error: %v", err)
	}
}

func TestListDevicesByOwnerUsernameForAdmin(t *testing.T) {
	store := newTestStore(t)

	alicePublic, err := store.CreateUser("alice", "alice@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bobPublic, err := store.CreateUser("bob", "bob@example.com", "secret1", false)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	alice, ok := store.GetUserByID(alicePublic.ID)
	if !ok {
		t.Fatal("alice not found")
	}
	bob, ok := store.GetUserByID(bobPublic.ID)
	if !ok {
		t.Fatal("bob not found")
	}
	admin, err := store.Authenticate("admin", "admin")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}

	if _, err := store.BindDevice("device-filter-a", alice.ID); err != nil {
		t.Fatalf("bind alice device: %v", err)
	}
	if _, err := store.BindDevice("device-filter-b", bob.ID); err != nil {
		t.Fatalf("bind bob device: %v", err)
	}

	devices := store.ListDevicesByOwnerUsername(admin, "ali")
	if len(devices) != 1 {
		t.Fatalf("unexpected filtered device count: %d", len(devices))
	}
	if devices[0].OwnerUsername != "alice" {
		t.Fatalf("unexpected owner username: %s", devices[0].OwnerUsername)
	}
}

func TestHandleHeartbeatDoesNotPersistVolatileDeviceState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), stateDBFilename)
	store := newTestStoreAtPath(t, dbPath)

	if _, err := store.HandleHeartbeat(ClientHeartbeatRequest{
		DeviceID:            "device-heartbeat-1",
		Hostname:            "host-1",
		SystemVersion:       "linux",
		OS:                  "linux",
		Arch:                "amd64",
		LocalIP:             "127.0.0.1",
		MAC:                 "00:11:22:33:44:55",
		CPUCount:            4,
		CPUPercent:          12.5,
		MemoryPercent:       30.2,
		NetworkOK:           true,
		OpenClawJSON:        `{"hello":"world"}`,
		OpenClawHash:        "hash-1",
		SyncIntervalSeconds: 30,
	}, "198.51.100.10"); err != nil {
		t.Fatalf("handle heartbeat: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened := newTestStoreAtPath(t, dbPath)
	admin, err := reopened.Authenticate("admin", "admin")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}

	devices := reopened.ListDevices(admin)
	if len(devices) != 1 {
		t.Fatalf("unexpected device count: %d", len(devices))
	}
	device := devices[0]
	if device.LastSeenAt != "" {
		t.Fatalf("last_seen_at should not be persisted: %s", device.LastSeenAt)
	}
	if device.SyncIntervalSeconds != 0 {
		t.Fatalf("sync_interval_seconds should not be persisted: %d", device.SyncIntervalSeconds)
	}
	if device.Status != (DeviceStatus{}) {
		t.Fatalf("device status should not be persisted: %+v", device.Status)
	}
	if device.OpenClawJSON != "" {
		t.Fatalf("device reported openclaw state should not be persisted")
	}
	if device.Online {
		t.Fatal("device should be offline after reload without heartbeat state")
	}
}

func TestSaveLockedRemovesExpiredSessionsAndPasswordResets(t *testing.T) {
	store := newTestStore(t)

	store.mu.Lock()
	store.state.Sessions = append(store.state.Sessions, Session{
		Token:     "expired-session",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	store.state.PasswordResets = append(store.state.PasswordResets, PasswordResetToken{
		Token:     "expired-reset",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if err := store.saveLocked(); err != nil {
		store.mu.Unlock()
		t.Fatalf("save locked: %v", err)
	}
	store.mu.Unlock()

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.state.Sessions) != 0 {
		t.Fatalf("expected expired sessions to be removed, got %d", len(store.state.Sessions))
	}
	if len(store.state.PasswordResets) != 0 {
		t.Fatalf("expected expired password resets to be removed, got %d", len(store.state.PasswordResets))
	}
}

func TestNewStoreMigratesLegacyJSONToSQLite(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, stateDBFilename)
	legacyPath := filepath.Join(tempDir, legacyStateFilename)

	legacyState := State{
		Settings: Settings{
			RegistrationEnabled: false,
		},
		Users: []User{
			{
				ID:           "legacy-user",
				Username:     "legacy",
				Email:        "legacy@example.com",
				PasswordSalt: "salt1",
				PasswordHash: "hash1",
				IsAdmin:      false,
				CreatedAt:    "2026-03-10T00:00:00Z",
				UpdatedAt:    "2026-03-10T00:00:00Z",
			},
		},
		Devices: []Device{
			{
				DeviceID:            "legacy-device",
				OwnerUserID:         "legacy-user",
				Remark:              "legacy remark",
				BoundAt:             "2026-03-10T00:00:00Z",
				LastSeenAt:          "2026-03-10T00:01:00Z",
				SyncIntervalSeconds: 30,
				Status: DeviceStatus{
					Hostname: "old-host",
				},
				OpenClawJSON:        `{"legacy":true}`,
				OpenClawHash:        "legacy-hash",
				PendingOpenClawJSON: `{"desired":true}`,
				PendingOpenClawHash: "desired-hash",
			},
		},
	}

	data, err := json.MarshalIndent(legacyState, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	store := newTestStoreAtPath(t, dbPath)
	if store.RegistrationEnabled() {
		t.Fatal("expected migrated registration setting to be false")
	}

	user, ok := store.GetUserByID("legacy-user")
	if !ok {
		t.Fatal("expected migrated user")
	}
	if user.Username != "legacy" {
		t.Fatalf("unexpected migrated username: %s", user.Username)
	}

	admin, err := store.Authenticate("admin", "admin")
	if err != nil {
		t.Fatalf("authenticate default admin: %v", err)
	}
	devices := store.ListDevices(admin)
	if len(devices) != 1 {
		t.Fatalf("unexpected migrated device count: %d", len(devices))
	}
	if devices[0].PendingOpenClawJSON != `{"desired":true}` {
		t.Fatalf("unexpected migrated pending config: %s", devices[0].PendingOpenClawJSON)
	}
	if devices[0].LastSeenAt != "" {
		t.Fatalf("volatile device state should not survive migration: %s", devices[0].LastSeenAt)
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite state file: %v", err)
	}
}
