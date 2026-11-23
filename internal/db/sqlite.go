package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database connection.
type Store struct {
	DB *sql.DB
}

// Open initializes the SQLite database with WAL mode and runs schema migrations.
func Open(path string) (*Store, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite allows a single writer at a time; serializing connections avoids SQLITE_BUSY.
	sqlDB.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA synchronous=NORMAL;",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("exec %q: %w", p, err)
		}
	}

	s := &Store{DB: sqlDB}
	if err := s.migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return s, nil
}

// Checkpoint flushes the WAL file into the main database file.
func (s *Store) Checkpoint() error {
	_, err := s.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	return err
}

// Close checkpoints and closes the database.
func (s *Store) Close() error {
	_ = s.Checkpoint()
	return s.DB.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS game_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	status TEXT NOT NULL DEFAULT 'running',
	started_at INTEGER NOT NULL,
	next_area_at INTEGER NOT NULL,
	forbidden_count INTEGER NOT NULL DEFAULT 1,
	hack_flag INTEGER NOT NULL DEFAULT 0,
	area_order TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	password_hash TEXT NOT NULL,
	first_name TEXT NOT NULL,
	last_name TEXT NOT NULL,
	sex TEXT NOT NULL,
	class_name TEXT NOT NULL,
	student_no INTEGER NOT NULL,
	club TEXT NOT NULL DEFAULT '',
	icon INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'ALIVE',
	tactics TEXT NOT NULL DEFAULT '보통',
	area_id INTEGER NOT NULL DEFAULT 0,
	att INTEGER NOT NULL,
	def INTEGER NOT NULL,
	hp INTEGER NOT NULL,
	max_hp INTEGER NOT NULL,
	level INTEGER NOT NULL DEFAULT 1,
	exp INTEGER NOT NULL DEFAULT 0,
	stamina INTEGER NOT NULL DEFAULT 300,
	kills INTEGER NOT NULL DEFAULT 0,
	last_action_at INTEGER NOT NULL,
	weapon_name TEXT NOT NULL DEFAULT '맨손',
	weapon_kind TEXT NOT NULL DEFAULT 'WN',
	weapon_att INTEGER NOT NULL DEFAULT 0,
	weapon_dur INTEGER NOT NULL DEFAULT 0,
	armor_body TEXT NOT NULL DEFAULT '없음',
	armor_body_def INTEGER NOT NULL DEFAULT 0,
	armor_body_dur INTEGER NOT NULL DEFAULT 0,
	armor_head TEXT NOT NULL DEFAULT '없음',
	armor_head_def INTEGER NOT NULL DEFAULT 0,
	armor_head_dur INTEGER NOT NULL DEFAULT 0,
	armor_foot TEXT NOT NULL DEFAULT '없음',
	armor_foot_def INTEGER NOT NULL DEFAULT 0,
	armor_foot_dur INTEGER NOT NULL DEFAULT 0,
	armor_arm TEXT NOT NULL DEFAULT '없음',
	armor_arm_def INTEGER NOT NULL DEFAULT 0,
	armor_arm_dur INTEGER NOT NULL DEFAULT 0,
	proficiency_json TEXT NOT NULL,
	battle_log TEXT NOT NULL DEFAULT '',
	death_msg TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	last_target_id TEXT NOT NULL DEFAULT '',
	poison_flag TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS user_items (
	user_id TEXT NOT NULL,
	slot INTEGER NOT NULL CHECK (slot BETWEEN 0 AND 5),
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	effect INTEGER NOT NULL,
	durability INTEGER NOT NULL,
	PRIMARY KEY (user_id, slot),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS area_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	area_id INTEGER NOT NULL,
	spawn_group INTEGER NOT NULL DEFAULT 1,
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	effect INTEGER NOT NULL,
	durability INTEGER NOT NULL,
	owner_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_area_items_area ON area_items(area_id);

CREATE TABLE IF NOT EXISTS news_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at INTEGER NOT NULL,
	kind TEXT NOT NULL,
	message TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS winners (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	round_ended_at INTEGER NOT NULL,
	winner_name TEXT NOT NULL,
	ending_type TEXT NOT NULL,
	kills INTEGER NOT NULL,
	summary_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
`
	if _, err := s.DB.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	return nil
}
