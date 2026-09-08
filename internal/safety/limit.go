package safety

import (
	"strings"
	"unicode"
)

// AddLimitIfNeeded 检查 SQL 是否需要追加 LIMIT 子句。
// 如果 sql 是 SELECT 语句且没有 LIMIT/UNION/INTO OUTFILE 等，则追加 LIMIT maxRows。
// 对于 SHOW/DESCRIBE/EXPLAIN 等非 SELECT 语句，或已有 LIMIT 的 SELECT，原样返回。
func AddLimitIfNeeded(sql string, maxRows int) string {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return sql
	}

	// 提取第一个关键字，判断语句类型
	firstKeyword := extractFirstKeyword(trimmed)

	// 只对 SELECT 语句追加 LIMIT
	if !strings.EqualFold(firstKeyword, "SELECT") {
		return sql
	}

	// 检查是否已有 LIMIT、UNION、INTO OUTFILE/DUMPFILE
	if hasLimitOrExclusion(trimmed) {
		return sql
	}

	// 截断到最后一个语句结束分号，避免 `...; LIMIT 1000` 造成语法错误
	sql = trimStatementTail(sql)

	// 追加 LIMIT
	return sql + " LIMIT " + itoa(maxRows)
}

// extractFirstKeyword 提取 SQL 的第一个关键字（忽略前导空白）。
func extractFirstKeyword(sql string) string {
	i := 0
	// 跳过前导空白
	for i < len(sql) && unicode.IsSpace(rune(sql[i])) {
		i++
	}
	// 提取关键字
	start := i
	for i < len(sql) && (unicode.IsLetter(rune(sql[i])) || unicode.IsDigit(rune(sql[i])) || sql[i] == '_') {
		i++
	}
	if start == i {
		return ""
	}
	return sql[start:i]
}

// hasLimitOrExclusion 检查 SQL 是否包含 LIMIT 关键字或排除条件（UNION、INTO OUTFILE 等）。
// 使用状态机跳过字符串和注释。
func hasLimitOrExclusion(sql string) bool {
	i := 0
	inString := false
	inLineComment := false
	inBlockComment := false
	stringChar := byte(0)

	for i < len(sql) {
		c := sql[i]

		// 注释标记的双字符前瞻
		if !inString && !inLineComment && i+1 < len(sql) {
			two := sql[i : i+2]
			if two == "/*" {
				inBlockComment = true
				i += 2
				continue
			}
			if two == "--" {
				inLineComment = true
				i += 2
				continue
			}
		}

		// 块注释内
		if inBlockComment {
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				inBlockComment = false
				i += 2
				continue
			}
			i++
			continue
		}

		// 行注释内
		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			i++
			continue
		}

		// 字符串内
		if inString {
			if c == stringChar {
				// 检查转义引号
				if i+1 < len(sql) && sql[i+1] == stringChar {
					i += 2
					continue
				}
				inString = false
				i++
				continue
			}
			i++
			continue
		}

		// 字符串开始
		if c == '\'' || c == '"' || c == '`' {
			inString = true
			stringChar = c
			i++
			continue
		}

		// 检查关键字
		if isWordByte(c) {
			start := i
			for i < len(sql) && isWordByte(sql[i]) {
				i++
			}
			word := strings.ToUpper(sql[start:i])

			// 检查排除条件
			switch word {
			case "LIMIT", "UNION":
				return true
			case "INTO":
				// 检查 INTO OUTFILE 或 INTO DUMPFILE
				if hasFollowingKeyword(sql[i:], "OUTFILE", "DUMPFILE") {
					return true
				}
			}
			continue
		}

		i++
	}

	return false
}

// trimStatementTail 截断 SQL 到最后一个不在字符串/注释内的分号处，
// 移除语句终止符及其后的内容（尾随注释、空白、多余分号）。
// 无分号时原样返回。
func trimStatementTail(sql string) string {
	i := 0
	inString := false
	inLineComment := false
	inBlockComment := false
	stringChar := byte(0)
	lastSemicolon := -1

	for i < len(sql) {
		c := sql[i]

		// 注释标记的双字符前瞻
		if !inString && !inLineComment && i+1 < len(sql) {
			two := sql[i : i+2]
			if two == "/*" {
				inBlockComment = true
				i += 2
				continue
			}
			if two == "--" {
				inLineComment = true
				i += 2
				continue
			}
		}

		// 块注释内
		if inBlockComment {
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				inBlockComment = false
				i += 2
				continue
			}
			i++
			continue
		}

		// 行注释内
		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			i++
			continue
		}

		// 字符串内
		if inString {
			if c == stringChar {
				// 检查转义引号
				if i+1 < len(sql) && sql[i+1] == stringChar {
					i += 2
					continue
				}
				inString = false
				i++
				continue
			}
			i++
			continue
		}

		// 字符串开始
		if c == '\'' || c == '"' || c == '`' {
			inString = true
			stringChar = c
			i++
			continue
		}

		// 语句结束分号
		if c == ';' {
			lastSemicolon = i
		}

		i++
	}

	if lastSemicolon < 0 {
		return sql
	}
	// 截断可能残留连续分号（如 `SELECT 1;;`），再收尾清理
	return strings.TrimSpace(strings.TrimRight(sql[:lastSemicolon], ";"))
}

// hasFollowingKeyword 检查跳过的空白后是否有指定的关键字之一。
func hasFollowingKeyword(remaining string, keywords ...string) bool {
	// 跳过空白
	i := 0
	for i < len(remaining) && unicode.IsSpace(rune(remaining[i])) {
		i++
	}
	if i >= len(remaining) {
		return false
	}

	// 提取下一个关键字
	start := i
	for i < len(remaining) && (unicode.IsLetter(rune(remaining[i])) || unicode.IsDigit(rune(remaining[i])) || remaining[i] == '_') {
		i++
	}
	if start == i {
		return false
	}
	nextWord := strings.ToUpper(remaining[start:i])

	for _, kw := range keywords {
		if nextWord == kw {
			return true
		}
	}
	return false
}

// isWordByte returns true if byte can be part of a keyword.
func isWordByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// itoa 将 int 转换为字符串（避免导入 strconv）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
