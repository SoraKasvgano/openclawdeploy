package backend

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

func openStateDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := configureStateDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initStateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configureStateDB(db *sql.DB) error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`PRAGMA busy_timeout = 5000;`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}

func initStateSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			registration_enabled INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			email TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_salt TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			is_admin INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);`,
		`CREATE TABLE IF NOT EXISTS password_resets (
			token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_user_id ON password_resets(user_id);`,
		`CREATE TABLE IF NOT EXISTS devices (
			device_id TEXT PRIMARY KEY,
			owner_user_id TEXT,
			remark TEXT NOT NULL DEFAULT '',
			bound_at TEXT NOT NULL DEFAULT '',
			pending_openclaw_json TEXT NOT NULL DEFAULT '',
			pending_openclaw_hash TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(owner_user_id) REFERENCES users(id) ON DELETE SET NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_devices_owner_user_id ON devices(owner_user_id);`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("init sqlite schema: %w", err)
		}
	}
	return nil
}

func (s *Store) loadStateLocked() error {
	empty, err := s.stateDBEmptyLocked()
	if err != nil {
		return err
	}
	if empty {
		migrated, err := s.loadLegacyStateLocked()
		if err != nil {
			return err
		}
		if migrated {
			if s.logger != nil {
				s.logger.Printf("migrated legacy state from %s to sqlite", legacyStatePath(s.path))
			}
			return nil
		}
	}

	state, err := s.loadStateFromDBLocked()
	if err != nil {
		return err
	}
	s.state = state
	return nil
}

func (s *Store) loadLegacyStateLocked() (bool, error) {
	path := legacyStatePath(s.path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read legacy state: %w", err)
	}

	state := State{
		Settings: Settings{
			RegistrationEnabled: true,
		},
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return false, fmt.Errorf("parse legacy state: %w", err)
	}

	s.state = state
	return true, nil
}

func (s *Store) loadStateFromDBLocked() (State, error) {
	state := State{
		Settings: Settings{
			RegistrationEnabled: true,
		},
	}

	if err := s.loadSettingsLocked(&state); err != nil {
		return State{}, err
	}
	if err := s.loadUsersLocked(&state); err != nil {
		return State{}, err
	}
	if err := s.loadSessionsLocked(&state); err != nil {
		return State{}, err
	}
	if err := s.loadPasswordResetsLocked(&state); err != nil {
		return State{}, err
	}
	if err := s.loadDevicesLocked(&state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Store) loadSettingsLocked(state *State) error {
	var enabled int
	err := s.db.QueryRow(`SELECT registration_enabled FROM settings WHERE id = 1`).Scan(&enabled)
	if err == nil {
		state.Settings.RegistrationEnabled = enabled != 0
		return nil
	}
	if err == sql.ErrNoRows {
		return nil
	}
	return fmt.Errorf("load settings: %w", err)
}

func (s *Store) loadUsersLocked(state *State) error {
	rows, err := s.db.Query(`
		SELECT id, username, email, password_salt, password_hash, is_admin, created_at, updated_at
		FROM users
		ORDER BY created_at, id
	`)
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		var isAdmin int
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordSalt, &user.PasswordHash, &isAdmin, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}
		user.IsAdmin = isAdmin != 0
		state.Users = append(state.Users, user)
	}
	return rows.Err()
}

func (s *Store) loadSessionsLocked(state *State) error {
	rows, err := s.db.Query(`
		SELECT token, user_id, expires_at
		FROM sessions
		ORDER BY expires_at, token
	`)
	if err != nil {
		return fmt.Errorf("load sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.Token, &session.UserID, &session.ExpiresAt); err != nil {
			return fmt.Errorf("scan session: %w", err)
		}
		state.Sessions = append(state.Sessions, session)
	}
	return rows.Err()
}

func (s *Store) loadPasswordResetsLocked(state *State) error {
	rows, err := s.db.Query(`
		SELECT token, user_id, expires_at
		FROM password_resets
		ORDER BY expires_at, token
	`)
	if err != nil {
		return fmt.Errorf("load password resets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var reset PasswordResetToken
		if err := rows.Scan(&reset.Token, &reset.UserID, &reset.ExpiresAt); err != nil {
			return fmt.Errorf("scan password reset: %w", err)
		}
		state.PasswordResets = append(state.PasswordResets, reset)
	}
	return rows.Err()
}

func (s *Store) loadDevicesLocked(state *State) error {
	rows, err := s.db.Query(`
		SELECT device_id, COALESCE(owner_user_id, ''), remark, bound_at, pending_openclaw_json, pending_openclaw_hash
		FROM devices
		ORDER BY bound_at, device_id
	`)
	if err != nil {
		return fmt.Errorf("load devices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var device Device
		if err := rows.Scan(&device.DeviceID, &device.OwnerUserID, &device.Remark, &device.BoundAt, &device.PendingOpenClawJSON, &device.PendingOpenClawHash); err != nil {
			return fmt.Errorf("scan device: %w", err)
		}
		state.Devices = append(state.Devices, device)
	}
	return rows.Err()
}

func (s *Store) stateDBEmptyLocked() (bool, error) {
	tables := []string{"settings", "users", "sessions", "password_resets", "devices"}
	for _, table := range tables {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		var count int
		if err := s.db.QueryRow(query).Scan(&count); err != nil {
			return false, fmt.Errorf("count %s: %w", table, err)
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

func (s *Store) saveStateLocked() error {
	persisted := s.persistedStateLocked()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, statement := range []string{
		`DELETE FROM password_resets`,
		`DELETE FROM sessions`,
		`DELETE FROM devices`,
		`DELETE FROM users`,
		`DELETE FROM settings`,
	} {
		if _, err = tx.Exec(statement); err != nil {
			return fmt.Errorf("clear sqlite tables: %w", err)
		}
	}

	if _, err = tx.Exec(`INSERT INTO settings (id, registration_enabled) VALUES (1, ?)`, boolToInt(persisted.Settings.RegistrationEnabled)); err != nil {
		return fmt.Errorf("insert settings: %w", err)
	}

	if err = insertUsers(tx, persisted.Users); err != nil {
		return err
	}
	if err = insertSessions(tx, persisted.Sessions); err != nil {
		return err
	}
	if err = insertPasswordResets(tx, persisted.PasswordResets); err != nil {
		return err
	}
	if err = insertDevices(tx, persisted.Devices); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite tx: %w", err)
	}
	return nil
}

func insertUsers(tx *sql.Tx, users []User) error {
	stmt, err := tx.Prepare(`
		INSERT INTO users (id, username, email, password_salt, password_hash, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare users insert: %w", err)
	}
	defer stmt.Close()

	for _, user := range users {
		if _, err := stmt.Exec(user.ID, user.Username, user.Email, user.PasswordSalt, user.PasswordHash, boolToInt(user.IsAdmin), user.CreatedAt, user.UpdatedAt); err != nil {
			return fmt.Errorf("insert user %s: %w", user.Username, err)
		}
	}
	return nil
}

func insertSessions(tx *sql.Tx, sessions []Session) error {
	stmt, err := tx.Prepare(`
		INSERT INTO sessions (token, user_id, expires_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare sessions insert: %w", err)
	}
	defer stmt.Close()

	for _, session := range sessions {
		if _, err := stmt.Exec(session.Token, session.UserID, session.ExpiresAt); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
	}
	return nil
}

func insertPasswordResets(tx *sql.Tx, resets []PasswordResetToken) error {
	stmt, err := tx.Prepare(`
		INSERT INTO password_resets (token, user_id, expires_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare password reset insert: %w", err)
	}
	defer stmt.Close()

	for _, reset := range resets {
		if _, err := stmt.Exec(reset.Token, reset.UserID, reset.ExpiresAt); err != nil {
			return fmt.Errorf("insert password reset: %w", err)
		}
	}
	return nil
}

func insertDevices(tx *sql.Tx, devices []Device) error {
	stmt, err := tx.Prepare(`
		INSERT INTO devices (device_id, owner_user_id, remark, bound_at, pending_openclaw_json, pending_openclaw_hash)
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare devices insert: %w", err)
	}
	defer stmt.Close()

	for _, device := range devices {
		if _, err := stmt.Exec(device.DeviceID, device.OwnerUserID, device.Remark, device.BoundAt, device.PendingOpenClawJSON, device.PendingOpenClawHash); err != nil {
			return fmt.Errorf("insert device %s: %w", device.DeviceID, err)
		}
	}
	return nil
}

func legacyStatePath(stateDBPath string) string {
	return filepath.Join(filepath.Dir(stateDBPath), legacyStateFilename)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
