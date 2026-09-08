package safety

import "testing"

func TestAddLimitIfNeeded(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		maxRows int
		want    string
	}{
		// SELECT 无 LIMIT → 追加
		{"simple select", "SELECT id FROM users", 1000, "SELECT id FROM users LIMIT 1000"},
		{"select with where", "SELECT * FROM users WHERE id > 10", 500, "SELECT * FROM users WHERE id > 10 LIMIT 500"},
		{"select with order by", "SELECT id FROM users ORDER BY id DESC", 100, "SELECT id FROM users ORDER BY id DESC LIMIT 100"},
		{"select with group by", "SELECT COUNT(*) FROM users GROUP BY status", 1000, "SELECT COUNT(*) FROM users GROUP BY status LIMIT 1000"},

		// SELECT 有 LIMIT → 保留原 LIMIT
		{"select with limit", "SELECT id FROM users LIMIT 100", 1000, "SELECT id FROM users LIMIT 100"},
		{"select with limit and offset", "SELECT id FROM users LIMIT 10 OFFSET 20", 1000, "SELECT id FROM users LIMIT 10 OFFSET 20"},
		{"select with limit comma offset", "SELECT id FROM users LIMIT 10, 20", 1000, "SELECT id FROM users LIMIT 10, 20"},

		// SELECT 有 UNION → 不加 LIMIT（复杂查询结构）
		{"select with union", "SELECT id FROM a UNION SELECT id FROM b", 1000, "SELECT id FROM a UNION SELECT id FROM b"},
		{"select with union all", "SELECT id FROM a UNION ALL SELECT id FROM b", 1000, "SELECT id FROM a UNION ALL SELECT id FROM b"},

		// 非 SELECT 语句 → 不加 LIMIT
		{"show databases", "SHOW DATABASES", 1000, "SHOW DATABASES"},
		{"show tables", "SHOW TABLES FROM mydb", 1000, "SHOW TABLES FROM mydb"},
		{"describe", "DESCRIBE users", 1000, "DESCRIBE users"},
		{"desc", "DESC users", 1000, "DESC users"},
		{"explain", "EXPLAIN SELECT id FROM users", 1000, "EXPLAIN SELECT id FROM users"},

		// SELECT INTO OUTFILE/DUMPFILE → 不加 LIMIT（虽然已被 safety 拦截）
		{"select into outfile", "SELECT * FROM users INTO OUTFILE '/tmp/x'", 1000, "SELECT * FROM users INTO OUTFILE '/tmp/x'"},
		{"select into dumpfile", "SELECT * FROM users INTO DUMPFILE '/tmp/x'", 1000, "SELECT * FROM users INTO DUMPFILE '/tmp/x'"},

		// 大小写不敏感
		{"uppercase limit", "SELECT id FROM users LIMIT 50", 1000, "SELECT id FROM users LIMIT 50"},
		{"lowercase limit", "select id from users limit 50", 1000, "select id from users limit 50"},
		{"mixed case select", "SeLeCt id FrOm users", 1000, "SeLeCt id FrOm users LIMIT 1000"},

		// 字符串字面量中的 LIMIT 不算
		{"limit in string", "SELECT 'LIMIT 100' FROM users", 1000, "SELECT 'LIMIT 100' FROM users LIMIT 1000"},
		{"limit in double quote", `SELECT "LIMIT" FROM users`, 1000, `SELECT "LIMIT" FROM users LIMIT 1000`},

		// 注释中的 LIMIT 不算
		{"limit in line comment", "SELECT id FROM users -- LIMIT 100", 1000, "SELECT id FROM users -- LIMIT 100 LIMIT 1000"},
		{"limit in block comment", "SELECT id FROM users /* LIMIT 100 */", 1000, "SELECT id FROM users /* LIMIT 100 */ LIMIT 1000"},

		// 边界情况
		{"select with subquery", "SELECT * FROM (SELECT id FROM users) AS t", 1000, "SELECT * FROM (SELECT id FROM users) AS t LIMIT 1000"},
		{"select with for update", "SELECT id FROM users FOR UPDATE", 1000, "SELECT id FROM users FOR UPDATE LIMIT 1000"},

		// 末尾分号：追加前截断，避免 `...; LIMIT 1000` 语法错误
		{"trailing semicolon", "SELECT id FROM users;", 1000, "SELECT id FROM users LIMIT 1000"},
		{"trailing double semicolon", "SELECT id FROM users;;", 1000, "SELECT id FROM users LIMIT 1000"},
		{"count aggregate with semicolon", "SELECT COUNT(*) FROM users;", 1000, "SELECT COUNT(*) FROM users LIMIT 1000"},
		{"semicolon then line comment", "SELECT id FROM users; -- done", 1000, "SELECT id FROM users LIMIT 1000"},
		{"semicolon then hash comment", "SELECT id FROM users; # done", 1000, "SELECT id FROM users LIMIT 1000"},
		{"semicolon inside string literal", "SELECT ';' FROM users;", 1000, "SELECT ';' FROM users LIMIT 1000"},
		{"semicolon inside backtick", "SELECT `a;b` FROM users;", 1000, "SELECT `a;b` FROM users LIMIT 1000"},
		{"semicolon inside block comment", "SELECT id FROM users /* ; */;", 1000, "SELECT id FROM users /* ; */ LIMIT 1000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AddLimitIfNeeded(tc.sql, tc.maxRows)
			if got != tc.want {
				t.Errorf("AddLimitIfNeeded(%q, %d)\n  got:  %q\n  want: %q", tc.sql, tc.maxRows, got, tc.want)
			}
		})
	}
}
