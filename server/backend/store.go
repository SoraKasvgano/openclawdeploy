package backend

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"openclawdeploy/internal/shared"
)

type UpdateUserInput struct {
	Username *string `json:"username"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	IsAdmin  *bool   `json:"is_admin"`
}

type Store struct {
	mu     sync.RWMutex
	path   string
	db     *sql.DB
	logger *log.Logger
	state  State
}

func NewStore(path string, logger *log.Logger) (*Store, error) {
	store := &Store{
		path:   path,
		logger: logger,
		state: State{
			Settings: Settings{
				RegistrationEnabled: true,
			},
		},
	}

	db, err := openStateDB(path)
	if err != nil {
		return nil, err
	}
	store.db = db

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.loadStateLocked(); err != nil {
		_ = store.db.Close()
		return nil, err
	}
	store.clearVolatileDeviceStateLocked()
	store.cleanupLocked(time.Now())
	if err := store.ensureAdminLocked(); err != nil {
		_ = store.db.Close()
		return nil, err
	}
	if err := store.saveLocked(); err != nil {
		_ = store.db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) RegistrationEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings.RegistrationEnabled
}

func (s *Store) SetRegistrationEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Settings.RegistrationEnabled = enabled
	return s.saveLocked()
}

func (s *Store) CreateUser(username, email, password string, isAdmin bool) (PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, err := s.createUserLocked(username, email, password, isAdmin)
	if err != nil {
		return PublicUser{}, err
	}
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, err
	}
	return s.publicUserLocked(user), nil
}

func (s *Store) createUserLocked(username, email, password string, isAdmin bool) (User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	if username == "" || email == "" || password == "" {
		return User{}, fmt.Errorf("username, email and password are required")
	}
	if len(password) < 6 {
		return User{}, fmt.Errorf("password must contain at least 6 characters")
	}

	for _, user := range s.state.Users {
		if strings.EqualFold(user.Username, username) {
			return User{}, fmt.Errorf("username already exists")
		}
		if strings.EqualFold(user.Email, email) {
			return User{}, fmt.Errorf("email already exists")
		}
	}

	salt, err := randomToken(12)
	if err != nil {
		return User{}, err
	}
	now := nowRFC3339()
	user := User{
		ID:           mustToken(10),
		Username:     username,
		Email:        email,
		PasswordSalt: salt,
		PasswordHash: hashPassword(salt, password),
		IsAdmin:      isAdmin,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.state.Users = append(s.state.Users, user)
	return user, nil
}

func (s *Store) Authenticate(username, password string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.state.Users {
		if strings.EqualFold(user.Username, strings.TrimSpace(username)) {
			if user.PasswordHash == hashPassword(user.PasswordSalt, strings.TrimSpace(password)) {
				return user, nil
			}
			break
		}
	}
	return User{}, fmt.Errorf("invalid username or password")
}

func (s *Store) CreateSession(userID string, ttl time.Duration) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	session := Session{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl).UTC().Format(time.RFC3339),
	}
	s.state.Sessions = append(s.state.Sessions, session)
	if err := s.saveLocked(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Store) GetUserBySession(token string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.cleanupLocked(now) {
		if err := s.saveLocked(); err != nil && s.logger != nil {
			s.logger.Printf("state cleanup save failed: %v", err)
		}
	}
	for _, session := range s.state.Sessions {
		if session.Token != token {
			continue
		}
		if expiry, err := time.Parse(time.RFC3339, session.ExpiresAt); err == nil && expiry.Before(now) {
			return User{}, false
		}
		for _, user := range s.state.Users {
			if user.ID == session.UserID {
				return user, true
			}
		}
	}
	return User{}, false
}

func (s *Store) DestroySession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.state.Sessions[:0]
	for _, session := range s.state.Sessions {
		if session.Token != token {
			next = append(next, session)
		}
	}
	s.state.Sessions = next
	return s.saveLocked()
}

func (s *Store) IssueResetToken(identifier string) (User, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	identifier = strings.TrimSpace(identifier)
	var target *User
	for index := range s.state.Users {
		user := &s.state.Users[index]
		if strings.EqualFold(user.Username, identifier) || strings.EqualFold(user.Email, identifier) {
			target = user
			break
		}
	}
	if target == nil {
		return User{}, "", fmt.Errorf("user not found")
	}

	token, err := randomToken(24)
	if err != nil {
		return User{}, "", err
	}
	s.state.PasswordResets = append(s.state.PasswordResets, PasswordResetToken{
		Token:     token,
		UserID:    target.ID,
		ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	if err := s.saveLocked(); err != nil {
		return User{}, "", err
	}
	return *target, token, nil
}

func (s *Store) ResetPassword(token, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	password = strings.TrimSpace(password)
	if len(password) < 6 {
		return fmt.Errorf("password must contain at least 6 characters")
	}

	now := time.Now()
	for index, reset := range s.state.PasswordResets {
		if reset.Token != token {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, reset.ExpiresAt)
		if err != nil || expiresAt.Before(now) {
			return fmt.Errorf("reset token expired")
		}
		for userIndex := range s.state.Users {
			if s.state.Users[userIndex].ID == reset.UserID {
				salt, err := randomToken(12)
				if err != nil {
					return err
				}
				s.state.Users[userIndex].PasswordSalt = salt
				s.state.Users[userIndex].PasswordHash = hashPassword(salt, password)
				s.state.Users[userIndex].UpdatedAt = nowRFC3339()

				s.state.PasswordResets = append(s.state.PasswordResets[:index], s.state.PasswordResets[index+1:]...)
				return s.saveLocked()
			}
		}
	}
	return fmt.Errorf("reset token invalid")
}

func (s *Store) ListUsers() []PublicUser {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]PublicUser, 0, len(s.state.Users))
	for _, user := range s.state.Users {
		result = append(result, s.publicUserLocked(user))
	}
	return result
}

func (s *Store) UpdateUser(id string, input UpdateUserInput) (PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *User
	for index := range s.state.Users {
		if s.state.Users[index].ID == id {
			target = &s.state.Users[index]
			break
		}
	}
	if target == nil {
		return PublicUser{}, fmt.Errorf("user not found")
	}

	if input.Username != nil {
		username := strings.TrimSpace(*input.Username)
		if username == "" {
			return PublicUser{}, fmt.Errorf("username cannot be empty")
		}
		for _, user := range s.state.Users {
			if user.ID != target.ID && strings.EqualFold(user.Username, username) {
				return PublicUser{}, fmt.Errorf("username already exists")
			}
		}
		target.Username = username
	}

	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == "" {
			return PublicUser{}, fmt.Errorf("email cannot be empty")
		}
		for _, user := range s.state.Users {
			if user.ID != target.ID && strings.EqualFold(user.Email, email) {
				return PublicUser{}, fmt.Errorf("email already exists")
			}
		}
		target.Email = email
	}

	if input.Password != nil && strings.TrimSpace(*input.Password) != "" {
		password := strings.TrimSpace(*input.Password)
		if len(password) < 6 {
			return PublicUser{}, fmt.Errorf("password must contain at least 6 characters")
		}
		salt, err := randomToken(12)
		if err != nil {
			return PublicUser{}, err
		}
		target.PasswordSalt = salt
		target.PasswordHash = hashPassword(salt, password)
	}

	if input.IsAdmin != nil {
		if !*input.IsAdmin && target.IsAdmin && s.adminCountLocked() == 1 {
			return PublicUser{}, fmt.Errorf("cannot remove the last admin")
		}
		target.IsAdmin = *input.IsAdmin
	}

	target.UpdatedAt = nowRFC3339()
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, err
	}
	return s.publicUserLocked(*target), nil
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var removed *User
	nextUsers := s.state.Users[:0]
	for _, user := range s.state.Users {
		if user.ID == id {
			u := user
			removed = &u
			continue
		}
		nextUsers = append(nextUsers, user)
	}
	if removed == nil {
		return fmt.Errorf("user not found")
	}
	if removed.IsAdmin && s.adminCountLocked() == 1 {
		return fmt.Errorf("cannot delete the last admin")
	}

	for index := range s.state.Devices {
		if s.state.Devices[index].OwnerUserID == removed.ID {
			s.state.Devices[index].OwnerUserID = ""
		}
	}
	nextSessions := s.state.Sessions[:0]
	for _, session := range s.state.Sessions {
		if session.UserID != removed.ID {
			nextSessions = append(nextSessions, session)
		}
	}
	s.state.Sessions = nextSessions

	nextResets := s.state.PasswordResets[:0]
	for _, reset := range s.state.PasswordResets {
		if reset.UserID != removed.ID {
			nextResets = append(nextResets, reset)
		}
	}
	s.state.PasswordResets = nextResets

	s.state.Users = nextUsers
	return s.saveLocked()
}

func (s *Store) GetUserByID(id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.state.Users {
		if user.ID == id {
			return user, true
		}
	}
	return User{}, false
}

func (s *Store) BindDevice(deviceID, userID string) (PublicDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deviceID = canonicalDeviceID(deviceID)
	if deviceID == "" {
		return PublicDevice{}, fmt.Errorf("device_id is required")
	}

	device := s.deviceByIDLocked(deviceID)
	if device == nil {
		s.state.Devices = append(s.state.Devices, Device{DeviceID: deviceID})
		device = &s.state.Devices[len(s.state.Devices)-1]
	}

	if device.OwnerUserID != "" && device.OwnerUserID != userID {
		return PublicDevice{}, fmt.Errorf("device already belongs to another user")
	}

	device.OwnerUserID = userID
	if device.BoundAt == "" {
		device.BoundAt = nowRFC3339()
	}
	if err := s.saveLocked(); err != nil {
		return PublicDevice{}, err
	}
	return s.publicDeviceLocked(*device), nil
}

func (s *Store) ListDevices(user User) []PublicDevice {
	return s.ListDevicesByOwnerUsername(user, "")
}

func (s *Store) ListDevicesByOwnerUsername(user User, ownerUsername string) []PublicDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filter := strings.ToLower(strings.TrimSpace(ownerUsername))
	devices := make([]PublicDevice, 0, len(s.state.Devices))
	for _, device := range s.state.Devices {
		if !user.IsAdmin && device.OwnerUserID != user.ID {
			continue
		}
		if user.IsAdmin && filter != "" {
			owner := strings.ToLower(s.usernameByIDLocked(device.OwnerUserID))
			if !strings.Contains(owner, filter) {
				continue
			}
		}
		devices = append(devices, s.publicDeviceLocked(device))
	}
	return devices
}

func (s *Store) DeleteDevice(deviceID string, user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deviceID = canonicalDeviceID(deviceID)
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}

	index := -1
	for deviceIndex := range s.state.Devices {
		if sameDeviceID(s.state.Devices[deviceIndex].DeviceID, deviceID) {
			index = deviceIndex
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("device not found")
	}

	device := s.state.Devices[index]
	if !user.IsAdmin && device.OwnerUserID != user.ID {
		return fmt.Errorf("permission denied")
	}

	s.state.Devices = append(s.state.Devices[:index], s.state.Devices[index+1:]...)
	return s.saveLocked()
}

func (s *Store) UpdateDeviceRemark(deviceID string, user User, remark string) (PublicDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device := s.deviceByIDLocked(deviceID)
	if device == nil {
		return PublicDevice{}, fmt.Errorf("device not found")
	}
	if !user.IsAdmin && device.OwnerUserID != user.ID {
		return PublicDevice{}, fmt.Errorf("permission denied")
	}

	device.Remark = strings.TrimSpace(remark)
	if err := s.saveLocked(); err != nil {
		return PublicDevice{}, err
	}
	return s.publicDeviceLocked(*device), nil
}

func (s *Store) UpdateDeviceConfig(deviceID string, user User, openclawJSON string) (PublicDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !json.Valid([]byte(strings.TrimSpace(openclawJSON))) {
		return PublicDevice{}, fmt.Errorf("openclaw.json must be valid JSON")
	}

	device := s.deviceByIDLocked(deviceID)
	if device == nil {
		return PublicDevice{}, fmt.Errorf("device not found")
	}
	if !user.IsAdmin && device.OwnerUserID != user.ID {
		return PublicDevice{}, fmt.Errorf("permission denied")
	}

	device.PendingOpenClawJSON = strings.TrimSpace(openclawJSON)
	device.PendingOpenClawHash = shared.HashString(device.PendingOpenClawJSON)
	if err := s.saveLocked(); err != nil {
		return PublicDevice{}, err
	}
	return s.publicDeviceLocked(*device), nil
}

func (s *Store) Summary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := Summary{
		UserCount:           len(s.state.Users),
		DeviceCount:         len(s.state.Devices),
		RegistrationEnabled: s.state.Settings.RegistrationEnabled,
	}
	for _, device := range s.state.Devices {
		if deviceOnline(device) {
			summary.OnlineDeviceCount++
		}
	}
	return summary
}

func (s *Store) HandleHeartbeat(request ClientHeartbeatRequest, externalIP string) (ClientHeartbeatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	request.DeviceID = canonicalDeviceID(request.DeviceID)
	if request.DeviceID == "" {
		return ClientHeartbeatResponse{}, fmt.Errorf("device_id is required")
	}

	device := s.deviceByIDLocked(request.DeviceID)
	persistentChanged := false
	if device == nil {
		s.state.Devices = append(s.state.Devices, Device{DeviceID: request.DeviceID})
		device = &s.state.Devices[len(s.state.Devices)-1]
		persistentChanged = true
	}

	device.LastSeenAt = nowRFC3339()
	device.SyncIntervalSeconds = request.SyncIntervalSeconds
	device.Status = DeviceStatus{
		Hostname:      request.Hostname,
		SystemVersion: request.SystemVersion,
		OS:            request.OS,
		Arch:          request.Arch,
		LocalIP:       request.LocalIP,
		ExternalIP:    externalIP,
		MAC:           request.MAC,
		CPUCount:      request.CPUCount,
		CPUPercent:    request.CPUPercent,
		MemoryPercent: request.MemoryPercent,
		NetworkOK:     request.NetworkOK,
	}
	device.OpenClawJSON = request.OpenClawJSON
	device.OpenClawHash = request.OpenClawHash

	if device.PendingOpenClawHash != "" && device.PendingOpenClawHash == request.OpenClawHash {
		device.PendingOpenClawJSON = ""
		device.PendingOpenClawHash = ""
		persistentChanged = true
	}

	response := ClientHeartbeatResponse{
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		Message:    "设备心跳已接收",
	}
	if device.PendingOpenClawJSON != "" && device.PendingOpenClawHash != device.OpenClawHash {
		response.ApplyConfig = true
		response.OpenClawJSON = device.PendingOpenClawJSON
		response.RestartOpenClaw = true
		response.Message = "服务端下发了新的 openclaw.json"
	}

	if persistentChanged {
		if err := s.saveLocked(); err != nil {
			return ClientHeartbeatResponse{}, err
		}
	}
	return response, nil
}

func (s *Store) ensureAdminLocked() error {
	for _, user := range s.state.Users {
		if user.IsAdmin {
			return nil
		}
	}

	salt, err := randomToken(12)
	if err != nil {
		return err
	}
	now := nowRFC3339()
	s.state.Users = append(s.state.Users, User{
		ID:           mustToken(10),
		Username:     "admin",
		Email:        "admin@local",
		PasswordSalt: salt,
		PasswordHash: hashPassword(salt, "admin"),
		IsAdmin:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	return nil
}

func (s *Store) publicUserLocked(user User) PublicUser {
	deviceCount := 0
	for _, device := range s.state.Devices {
		if device.OwnerUserID == user.ID {
			deviceCount++
		}
	}
	return PublicUser{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		IsAdmin:     user.IsAdmin,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		DeviceCount: deviceCount,
	}
}

func (s *Store) publicDeviceLocked(device Device) PublicDevice {
	return PublicDevice{
		DeviceID:            canonicalDeviceID(device.DeviceID),
		OwnerUserID:         device.OwnerUserID,
		OwnerUsername:       s.usernameByIDLocked(device.OwnerUserID),
		Remark:              device.Remark,
		BoundAt:             device.BoundAt,
		LastSeenAt:          device.LastSeenAt,
		SyncIntervalSeconds: device.SyncIntervalSeconds,
		Online:              deviceOnline(device),
		Status:              device.Status,
		OpenClawJSON:        device.OpenClawJSON,
		PendingOpenClawJSON: device.PendingOpenClawJSON,
	}
}

func (s *Store) deviceByIDLocked(deviceID string) *Device {
	deviceID = canonicalDeviceID(deviceID)
	for index := range s.state.Devices {
		if sameDeviceID(s.state.Devices[index].DeviceID, deviceID) {
			normalized := canonicalDeviceID(s.state.Devices[index].DeviceID)
			if normalized != "" {
				s.state.Devices[index].DeviceID = normalized
			}
			return &s.state.Devices[index]
		}
	}
	return nil
}

func (s *Store) usernameByIDLocked(userID string) string {
	for _, user := range s.state.Users {
		if user.ID == userID {
			return user.Username
		}
	}
	return ""
}

func (s *Store) adminCountLocked() int {
	count := 0
	for _, user := range s.state.Users {
		if user.IsAdmin {
			count++
		}
	}
	return count
}

func (s *Store) cleanupLocked(now time.Time) bool {
	changed := false
	validUsers := make(map[string]struct{}, len(s.state.Users))
	for _, user := range s.state.Users {
		validUsers[user.ID] = struct{}{}
	}

	nextSessions := s.state.Sessions[:0]
	for _, session := range s.state.Sessions {
		expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
		_, userExists := validUsers[session.UserID]
		if err == nil && expiresAt.After(now) && userExists {
			nextSessions = append(nextSessions, session)
			continue
		}
		changed = true
	}
	s.state.Sessions = nextSessions

	nextResets := s.state.PasswordResets[:0]
	for _, reset := range s.state.PasswordResets {
		expiresAt, err := time.Parse(time.RFC3339, reset.ExpiresAt)
		_, userExists := validUsers[reset.UserID]
		if err == nil && expiresAt.After(now) && userExists {
			nextResets = append(nextResets, reset)
			continue
		}
		changed = true
	}
	s.state.PasswordResets = nextResets

	for index := range s.state.Devices {
		ownerID := s.state.Devices[index].OwnerUserID
		if ownerID == "" {
			continue
		}
		if _, ok := validUsers[ownerID]; ok {
			continue
		}
		s.state.Devices[index].OwnerUserID = ""
		changed = true
	}

	return changed
}

func (s *Store) saveLocked() error {
	s.cleanupLocked(time.Now())
	return s.saveStateLocked()
}

func (s *Store) persistedStateLocked() State {
	persisted := s.state
	persisted.Devices = make([]Device, len(s.state.Devices))
	for index, device := range s.state.Devices {
		sanitized := device
		sanitized.LastSeenAt = ""
		sanitized.SyncIntervalSeconds = 0
		sanitized.Status = DeviceStatus{}
		sanitized.OpenClawJSON = ""
		sanitized.OpenClawHash = ""
		persisted.Devices[index] = sanitized
	}
	return persisted
}

func (s *Store) clearVolatileDeviceStateLocked() {
	for index := range s.state.Devices {
		s.state.Devices[index].LastSeenAt = ""
		s.state.Devices[index].SyncIntervalSeconds = 0
		s.state.Devices[index].Status = DeviceStatus{}
		s.state.Devices[index].OpenClawJSON = ""
		s.state.Devices[index].OpenClawHash = ""
	}
}

func deviceOnline(device Device) bool {
	if device.LastSeenAt == "" {
		return false
	}
	lastSeenAt, err := time.Parse(time.RFC3339, device.LastSeenAt)
	if err != nil {
		return false
	}
	timeout := 90 * time.Second
	if device.SyncIntervalSeconds > 0 {
		timeout = time.Duration(device.SyncIntervalSeconds*2+10) * time.Second
	}
	return time.Since(lastSeenAt) <= timeout
}

func hashPassword(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func randomToken(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func mustToken(length int) string {
	token, err := randomToken(length)
	if err != nil {
		panic(err)
	}
	return token
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func canonicalDeviceID(deviceID string) string {
	trimmed := strings.TrimSpace(deviceID)
	if normalized := shared.NormalizeDeviceID(trimmed); normalized != "" {
		return normalized
	}
	return trimmed
}

func sameDeviceID(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}

	leftNormalized := shared.NormalizeDeviceID(left)
	rightNormalized := shared.NormalizeDeviceID(right)
	if leftNormalized != "" && rightNormalized != "" {
		return leftNormalized == rightNormalized
	}

	return left == right
}
