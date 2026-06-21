// SQLite 存储层实现
// 使用 WAL 模式 + 按小时分表 + 预聚合表，支持不设硬限的规模化
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-262144&_foreign_keys=OFF")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写者
	db.SetMaxIdleConns(1)

	s := &SQLiteStore{db: db}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// InitSchema 初始化所有元数据表
func (s *SQLiteStore) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS admin (
		id          INTEGER PRIMARY KEY CHECK(id = 1),
		password    TEXT NOT NULL DEFAULT '',
		totp_secret TEXT NOT NULL DEFAULT ''
	);
	INSERT OR IGNORE INTO admin (id) VALUES (1);

	CREATE TABLE IF NOT EXISTS agents (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL DEFAULT '',
		hostname     TEXT NOT NULL DEFAULT '',
		group_id     TEXT REFERENCES groups(id),
		secret       TEXT NOT NULL,
		os_version   TEXT NOT NULL DEFAULT '',
		agent_ver    TEXT NOT NULL DEFAULT '',
		arch         TEXT NOT NULL DEFAULT '',
		collect_intv INTEGER,
		ping_intv    INTEGER,
		online       INTEGER NOT NULL DEFAULT 0,
		last_seen_at TIMESTAMP,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS groups (
		id             TEXT PRIMARY KEY,
		name           TEXT NOT NULL UNIQUE,
		collect_intv   INTEGER,
		ping_intv      INTEGER,
		telegram_conf_id INTEGER
	);

	CREATE TABLE IF NOT EXISTS isp_targets (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		name    TEXT NOT NULL,
		ip      TEXT NOT NULL,
		port    INTEGER NOT NULL DEFAULT 80,
		mode    TEXT NOT NULL DEFAULT 'auto',
		enabled INTEGER NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id    TEXT NOT NULL REFERENCES agents(id),
		metric      TEXT NOT NULL,
		threshold   REAL NOT NULL DEFAULT 0,
		value       REAL NOT NULL DEFAULT 0,
		status      TEXT NOT NULL DEFAULT 'firing',
		fired_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		resolved_at TIMESTAMP,
		notified    INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS install_tokens (
		token      TEXT PRIMARY KEY,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		used_at    TIMESTAMP,
		agent_id   TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_alerts_agent_metric ON alerts(agent_id, metric);
	CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);

	-- ping_agg_1min 预聚合表
	CREATE TABLE IF NOT EXISTS ping_agg_1min (
		bucket_min INTEGER NOT NULL,
		agent_id   TEXT NOT NULL,
		isp        TEXT NOT NULL,
		cnt        INTEGER NOT NULL DEFAULT 0,
		avg_lat    REAL NOT NULL DEFAULT 0,
		min_lat    REAL NOT NULL DEFAULT 0,
		max_lat    REAL NOT NULL DEFAULT 0,
		sum_lat    REAL NOT NULL DEFAULT 0,
		loss_cnt   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (agent_id, isp, bucket_min)
	) WITHOUT ROWID;
	`
	_, err := s.db.Exec(schema)
	return err
}

// ---- 管理员 ----

func (s *SQLiteStore) SetAdminPassword(hash string) error {
	_, err := s.db.Exec("UPDATE admin SET password = ? WHERE id = 1", hash)
	return err
}

func (s *SQLiteStore) GetAdminPassword() (string, error) {
	var hash string
	err := s.db.QueryRow("SELECT password FROM admin WHERE id = 1").Scan(&hash)
	return hash, err
}

func (s *SQLiteStore) SetTOTPSecret(secret string) error {
	_, err := s.db.Exec("UPDATE admin SET totp_secret = ? WHERE id = 1", secret)
	return err
}

func (s *SQLiteStore) GetTOTPSecret() (string, error) {
	var secret string
	err := s.db.QueryRow("SELECT totp_secret FROM admin WHERE id = 1").Scan(&secret)
	return secret, err
}

// ---- 探针注册 ----

func (s *SQLiteStore) CreateInstallToken() (string, error) {
	token := "token-" + randomHex(32)
	_, err := s.db.Exec(
		"INSERT INTO install_tokens (token, expires_at) VALUES (?, datetime('now', '+30 minutes'))",
		token,
	)
	return token, err
}

func (s *SQLiteStore) ConsumeInstallToken(token string) (bool, error) {
	// 使用事务确保一次性
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var expiresAt time.Time
	var usedAt *time.Time
	err = tx.QueryRow("SELECT expires_at, used_at FROM install_tokens WHERE token = ?", token).Scan(&expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if usedAt != nil {
		return false, nil // 已使用
	}
	if time.Now().After(expiresAt) {
		return false, nil // 已过期
	}
	now := time.Now()
	_, err = tx.Exec("UPDATE install_tokens SET used_at = ? WHERE token = ?", now, token)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// RegisterAgent 用 token 注册探针，返回 Agent 信息和个体凭证
func (s *SQLiteStore) RegisterAgent(token, hostname, agentVer, arch string) (*Agent, string, error) {
	ok, err := s.ConsumeInstallToken(token)
	if err != nil {
		return nil, "", fmt.Errorf("验证 token 失败: %w", err)
	}
	if !ok {
		return nil, "", fmt.Errorf("token 无效或已过期")
	}

	agentID := newUUID()
	agentSecret := randomHex(32)
	secretHash, err := bcrypt.GenerateFromPassword([]byte(agentSecret), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("生成密码哈希失败: %w", err)
	}

	now := time.Now()
	_, err = s.db.Exec(
		`INSERT INTO agents (id, hostname, agent_ver, arch, secret, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		agentID, hostname, agentVer, arch, string(secretHash), now, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("插入探针记录失败: %w", err)
	}

	agent := &Agent{
		ID:        agentID,
		Hostname:  hostname,
		AgentVer:  agentVer,
		Arch:      arch,
		CreatedAt: now,
	}
	return agent, agentSecret, nil
}

func (s *SQLiteStore) ValidateAgent(agentID, secret string) bool {
	var hash string
	err := s.db.QueryRow("SELECT secret FROM agents WHERE id = ?", agentID).Scan(&hash)
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(
		`SELECT id, name, hostname, group_id, os_version, agent_ver, arch,
		        collect_intv, ping_intv, online, last_seen_at, created_at, updated_at
		 FROM agents WHERE id = ?`, id)
	a := &Agent{}
	var groupID, osVer, agentVer, arch, hostname sql.NullString
	var collectIntv, pingIntv sql.NullInt64
	var lastSeen sql.NullTime
	err := row.Scan(&a.ID, &a.Name, &hostname, &groupID, &osVer, &agentVer, &arch,
		&collectIntv, &pingIntv, &a.Online, &lastSeen, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if hostname.Valid { a.Hostname = hostname.String }
	if groupID.Valid { a.GroupID = &groupID.String }
	if osVer.Valid { a.OSVersion = osVer.String }
	if agentVer.Valid { a.AgentVer = agentVer.String }
	if arch.Valid { a.Arch = arch.String }
	if collectIntv.Valid { v := int(collectIntv.Int64); a.CollectIntv = &v }
	if pingIntv.Valid { v := int(pingIntv.Int64); a.PingIntv = &v }
	if lastSeen.Valid { a.LastSeenAt = &lastSeen.Time }

	// 如果 name 为空就用 hostname
	if a.Name == "" {
		a.Name = a.Hostname
	}
	return a, nil
}

func (s *SQLiteStore) ListAgents() ([]*Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, hostname, group_id, os_version, agent_ver, arch,
		        collect_intv, ping_intv, online, last_seen_at, created_at, updated_at
		 FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		var groupID, osVer, agentVer, arch, hostname sql.NullString
		var collectIntv, pingIntv sql.NullInt64
		var lastSeen sql.NullTime
		err := rows.Scan(&a.ID, &a.Name, &hostname, &groupID, &osVer, &agentVer, &arch,
			&collectIntv, &pingIntv, &a.Online, &lastSeen, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if hostname.Valid { a.Hostname = hostname.String }
		if groupID.Valid { a.GroupID = &groupID.String }
		if osVer.Valid { a.OSVersion = osVer.String }
		if agentVer.Valid { a.AgentVer = agentVer.String }
		if arch.Valid { a.Arch = arch.String }
		if collectIntv.Valid { v := int(collectIntv.Int64); a.CollectIntv = &v }
		if pingIntv.Valid { v := int(pingIntv.Int64); a.PingIntv = &v }
		if lastSeen.Valid { a.LastSeenAt = &lastSeen.Time }
		if a.Name == "" { a.Name = a.Hostname }
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *SQLiteStore) UpdateAgent(agent *Agent) error {
	agent.UpdatedAt = time.Now()
	_, err := s.db.Exec(
		`UPDATE agents SET name=?, group_id=?, os_version=?, agent_ver=?,
		 arch=?, collect_intv=?, ping_intv=?, updated_at=?
		 WHERE id=?`,
		agent.Name, agent.GroupID, agent.OSVersion, agent.AgentVer,
		agent.Arch, agent.CollectIntv, agent.PingIntv, agent.UpdatedAt, agent.ID)
	return err
}

func (s *SQLiteStore) DeleteAgent(id string) error {
	_, err := s.db.Exec("DELETE FROM agents WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) SetAgentOnline(id string, online bool, seenAt time.Time) error {
	on := 0
	if online { on = 1 }
	_, err := s.db.Exec(
		"UPDATE agents SET online = ?, last_seen_at = ? WHERE id = ?",
		on, seenAt, id)
	return err
}

// ---- 分组管理 ----

func (s *SQLiteStore) ListGroups() ([]*Group, error) {
	rows, err := s.db.Query("SELECT id, name, collect_intv, ping_intv, telegram_conf_id FROM groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []*Group
	for rows.Next() {
		g := &Group{}
		var ci, pi sql.NullInt64
		var tc sql.NullInt64
		err := rows.Scan(&g.ID, &g.Name, &ci, &pi, &tc)
		if err != nil { return nil, err }
		if ci.Valid { v := int(ci.Int64); g.CollectIntv = &v }
		if pi.Valid { v := int(pi.Int64); g.PingIntv = &v }
		if tc.Valid { g.TelegramConfID = &tc.Int64 }
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *SQLiteStore) GetGroup(id string) (*Group, error) {
	row := s.db.QueryRow("SELECT id, name, collect_intv, ping_intv, telegram_conf_id FROM groups WHERE id = ?", id)
	g := &Group{}
	var ci, pi sql.NullInt64
	var tc sql.NullInt64
	err := row.Scan(&g.ID, &g.Name, &ci, &pi, &tc)
	if err != nil { return nil, err }
	if ci.Valid { v := int(ci.Int64); g.CollectIntv = &v }
	if pi.Valid { v := int(pi.Int64); g.PingIntv = &v }
	if tc.Valid { g.TelegramConfID = &tc.Int64 }
	return g, nil
}

func (s *SQLiteStore) CreateGroup(name string) (*Group, error) {
	id := newUUID()
	_, err := s.db.Exec("INSERT INTO groups (id, name) VALUES (?, ?)", id, name)
	if err != nil {
		return nil, fmt.Errorf("创建分组 %s 失败: %w", name, err)
	}
	return &Group{ID: id, Name: name}, nil
}

func (s *SQLiteStore) UpdateGroup(group *Group) error {
	_, err := s.db.Exec(
		"UPDATE groups SET name=?, collect_intv=?, ping_intv=?, telegram_conf_id=? WHERE id=?",
		group.Name, group.CollectIntv, group.PingIntv, group.TelegramConfID, group.ID)
	return err
}

func (s *SQLiteStore) DeleteGroup(id string) error {
	// 将属于该组的探针移到未分组
	_, err := s.db.Exec("UPDATE agents SET group_id = NULL WHERE group_id = ?", id)
	if err != nil { return err }
	_, err = s.db.Exec("DELETE FROM groups WHERE id = ?", id)
	return err
}

// ---- ISP ----

func (s *SQLiteStore) ListISPTargets() ([]*ISPTarget, error) {
	rows, err := s.db.Query("SELECT id, name, ip, port, mode, enabled FROM isp_targets WHERE enabled = 1")
	if err != nil { return nil, err }
	defer rows.Close()
	var targets []*ISPTarget
	for rows.Next() {
		t := &ISPTarget{}
		err := rows.Scan(&t.ID, &t.Name, &t.IP, &t.Port, &t.Mode, &t.Enabled)
		if err != nil { return nil, err }
		targets = append(targets, t)
	}
	return targets, nil
}

func (s *SQLiteStore) CreateISPTarget(target *ISPTarget) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO isp_targets (name, ip, port, mode, enabled) VALUES (?, ?, ?, ?, ?)",
		target.Name, target.IP, target.Port, target.Mode, target.Enabled)
	if err != nil { return 0, err }
	return res.LastInsertId()
}

func (s *SQLiteStore) UpdateISPTarget(target *ISPTarget) error {
	_, err := s.db.Exec(
		"UPDATE isp_targets SET name=?, ip=?, port=?, mode=?, enabled=? WHERE id=?",
		target.Name, target.IP, target.Port, target.Mode, target.Enabled, target.ID)
	return err
}

func (s *SQLiteStore) DeleteISPTarget(id int64) error {
	_, err := s.db.Exec("DELETE FROM isp_targets WHERE id = ?", id)
	return err
}

// ---- 设置 ----

func (s *SQLiteStore) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows { return "", nil }
	return value, err
}

func (s *SQLiteStore) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

// ---- 时序数据写入 ----

// tableNameForTime 获取系统指标表名（按小时分表）
func tableNameForTime(prefix string, ts time.Time) string {
	return fmt.Sprintf("%s_%s", prefix, ts.Format("2006010215"))
}

func (s *SQLiteStore) WriteSystemMetric(agentID string, ts time.Time, cpu, mem, disk float64, netUp, netDown int64, osVersion string) error {
	table := tableNameForTime("metrics_sys", ts)
	// 使用 CREATE TABLE IF NOT EXISTS 按需创建小时表
	createSQL := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			ts INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			cpu REAL,
			mem REAL,
			disk REAL,
			net_up INTEGER,
			net_down INTEGER,
			os_version TEXT
		)`, table)
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("创建小时表 %s 失败: %w", table, err)
	}
	// 创建索引（IF NOT EXISTS）
	indexSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_agent_ts ON %s(agent_id, ts)", table, table)
	s.db.Exec(indexSQL)

	_, err := s.db.Exec(
		fmt.Sprintf("INSERT INTO %s (ts, agent_id, cpu, mem, disk, net_up, net_down, os_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", table),
		ts.Unix(), agentID, cpu, mem, disk, netUp, netDown, osVersion)
	return err
}

func (s *SQLiteStore) WritePingMetric(agentID string, ts time.Time, isp, targetIP string, latency, loss, jitter float64) error {
	table := tableNameForTime("metrics_ping", ts)
	createSQL := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			ts INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			isp TEXT NOT NULL,
			target_ip TEXT NOT NULL,
			latency REAL,
			loss REAL,
			jitter REAL
		)`, table)
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("创建 Ping 小时表 %s 失败: %w", table, err)
	}
	indexSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_agent_isp_ts ON %s(agent_id, isp, ts)", table, table)
	s.db.Exec(indexSQL)

	_, err := s.db.Exec(
		fmt.Sprintf("INSERT INTO %s (ts, agent_id, isp, target_ip, latency, loss, jitter) VALUES (?, ?, ?, ?, ?, ?, ?)", table),
		ts.Unix(), agentID, isp, targetIP, latency, loss, jitter)
	return err
}

// AggregatePingMin 聚合上一分钟的 Ping 数据到预聚合表
func (s *SQLiteStore) AggregatePingMin() error {
	now := time.Now().Truncate(time.Minute)
	bucket := now.Unix() - 60 // 上一分钟
	table := tableNameForTime("metrics_ping", now.Add(-time.Minute))

	// 从原始表聚合，写入预聚合表
	sql := fmt.Sprintf(`
		INSERT OR REPLACE INTO ping_agg_1min (bucket_min, agent_id, isp, cnt, avg_lat, min_lat, max_lat, sum_lat, loss_cnt)
		SELECT ?, agent_id, isp, COUNT(*), AVG(latency), MIN(latency), MAX(latency), SUM(latency),
		       SUM(CASE WHEN loss > 0 THEN 1 ELSE 0 END)
		FROM %s
		WHERE ts >= ? AND ts < ?
		GROUP BY agent_id, isp`, table)

	_, err := s.db.Exec(sql, bucket, bucket, bucket+60)
	// 即使原始表不存在也可忽略
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	return nil
}

// ---- 时序数据查询 ----

func (s *SQLiteStore) GetLatestMetrics(agentID string) (*LatestMetric, error) {
	// 从最新的小时表查最新一条
	now := time.Now()
	for i := 0; i < 2; i++ {
		table := tableNameForTime("metrics_sys", now.Add(-time.Duration(i)*time.Hour))
		row := s.db.QueryRow(
			fmt.Sprintf("SELECT cpu, mem, disk, net_up, net_down, os_version FROM %s WHERE agent_id = ? ORDER BY ts DESC LIMIT 1", table),
			agentID)
		m := &LatestMetric{AgentID: agentID}
		var osVer sql.NullString
		err := row.Scan(&m.CPU, &m.Mem, &m.Disk, &m.NetUp, &m.NetDown, &osVer)
		if err == nil {
			if osVer.Valid { m.OSVersion = osVer.String }
			return m, nil
		}
	}
	return nil, fmt.Errorf("未找到探针 %s 的指标数据", agentID)
}

func (s *SQLiteStore) GetAllLatestMetrics() (map[string]*LatestMetric, error) {
	// 优化：不从 SQLite 全量查，由调用方维护内存 map
	// 这里作为兜底，从最近的小时表全量查（不设硬限下可能慢）
	now := time.Now()
	table := tableNameForTime("metrics_sys", now)
	rows, err := s.db.Query(
		fmt.Sprintf("SELECT agent_id, cpu, mem, disk, net_up, net_down, os_version, MAX(ts) FROM %s GROUP BY agent_id", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*LatestMetric)
	for rows.Next() {
		m := &LatestMetric{}
		var osVer sql.NullString
		if err := rows.Scan(&m.AgentID, &m.CPU, &m.Mem, &m.Disk, &m.NetUp, &m.NetDown, &osVer); err == nil {
			if osVer.Valid { m.OSVersion = osVer.String }
			result[m.AgentID] = m
		}
	}
	return result, nil
}

func (s *SQLiteStore) GetPingAgg(agentID, isp string, since, until time.Time) ([]*PingAggMin, error) {
	rows, err := s.db.Query(
		`SELECT bucket_min, cnt, avg_lat, min_lat, max_lat, loss_cnt FROM ping_agg_1min
		 WHERE agent_id = ? AND isp = ? AND bucket_min >= ? AND bucket_min < ?
		 ORDER BY bucket_min`,
		agentID, isp, since.Unix(), until.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*PingAggMin
	for rows.Next() {
		p := &PingAggMin{AgentID: agentID, ISP: isp}
		var bucket int64
		var lossCnt int
		err := rows.Scan(&bucket, &p.Count, &p.AvgLat, &p.MinLat, &p.MaxLat, &lossCnt)
		if err != nil { return nil, err }
		p.BucketMin = time.Unix(bucket, 0)
		p.LossRate = float64(lossCnt) / float64(p.Count)
		results = append(results, p)
	}
	return results, nil
}

func (s *SQLiteStore) GetSystemMetrics(agentID string, since, until time.Time) ([]*RawSystemMetric, error) {
	// 跨小时表的 UNION ALL 查询
	var tables []string
	for t := since.Truncate(time.Hour); !t.After(until); t = t.Add(time.Hour) {
		table := tableNameForTime("metrics_sys", t)
		tables = append(tables, fmt.Sprintf(
			"SELECT ts, cpu, mem, disk, net_up, net_down FROM %s WHERE agent_id = ? AND ts >= ? AND ts < ?",
			table))
	}
	if len(tables) == 0 {
		return nil, nil
	}
	query := strings.Join(tables, " UNION ALL ") + " ORDER BY ts"
	rows, err := s.db.Query(query, agentID, since.Unix(), until.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*RawSystemMetric
	for rows.Next() {
		r := &RawSystemMetric{}
		var ts int64
		err := rows.Scan(&ts, &r.CPU, &r.Mem, &r.Disk, &r.NetUp, &r.NetDown)
		if err != nil { return nil, err }
		r.Timestamp = time.Unix(ts, 0)
		results = append(results, r)
	}
	return results, nil
}

// ---- 告警 ----

func (s *SQLiteStore) CreateAlert(alert *Alert) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO alerts (agent_id, metric, threshold, value, status, fired_at, notified)
		 VALUES (?, ?, ?, ?, 'firing', ?, 0)`,
		alert.AgentID, alert.Metric, alert.Threshold, alert.Value, alert.FiredAt)
	if err != nil { return 0, err }
	return res.LastInsertId()
}

func (s *SQLiteStore) ResolveAlert(agentID, metric string) error {
	now := time.Now()
	_, err := s.db.Exec(
		"UPDATE alerts SET status='resolved', resolved_at=? WHERE agent_id=? AND metric=? AND status='firing'",
		now, agentID, metric)
	return err
}

func (s *SQLiteStore) ListActiveAlerts() ([]*Alert, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, metric, threshold, value, fired_at, notified
		 FROM alerts WHERE status = 'firing'`)
	if err != nil { return nil, err }
	defer rows.Close()
	var alerts []*Alert
	for rows.Next() {
		a := &Alert{Status: "firing"}
		err := rows.Scan(&a.ID, &a.AgentID, &a.Metric, &a.Threshold, &a.Value, &a.FiredAt, &a.Notified)
		if err != nil { return nil, err }
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (s *SQLiteStore) GetActiveAlert(agentID, metric string) (*Alert, error) {
	row := s.db.QueryRow(
		"SELECT id, agent_id, metric, threshold, value, fired_at, notified FROM alerts WHERE agent_id=? AND metric=? AND status='firing'",
		agentID, metric)
	a := &Alert{Status: "firing"}
	err := row.Scan(&a.ID, &a.AgentID, &a.Metric, &a.Threshold, &a.Value, &a.FiredAt, &a.Notified)
	if err != nil { return nil, err }
	return a, nil
}

// ---- 安装 Token ----

// ---- 数据维护 ----

func (s *SQLiteStore) DropOldHourlyTables(keepHours int) error {
	// 查出所有 metrics_sys_ 和 metrics_ping_ 表
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' AND (name LIKE 'metrics_sys_%' OR name LIKE 'metrics_ping_%')")
	if err != nil { return err }
	defer rows.Close()

	cutoff := time.Now().Add(-time.Duration(keepHours) * time.Hour)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil { continue }
		// 解析小时表名中的时间戳
		// metrics_sys_2026062114 -> 14 = 2026-06-21 14:00
		timeStr := strings.TrimPrefix(strings.TrimPrefix(name, "metrics_sys_"), "metrics_ping_")
		t, err := time.ParseInLocation("2006010215", timeStr, time.Local)
		if err != nil { continue }
		if t.Before(cutoff) {
			_, err := s.db.Exec("DROP TABLE IF EXISTS " + name)
			if err != nil {
				log.Printf("删除旧表 %s 失败: %v", name, err)
			} else {
				log.Printf("已删除过期小时表: %s", name)
			}
		}
	}
	return nil
}

func (s *SQLiteStore) CleanOldAggData(hours int) error {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	_, err := s.db.Exec("DELETE FROM ping_agg_1min WHERE bucket_min < ?", cutoff)
	return err
}

// ---- 辅助函数 ----

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}