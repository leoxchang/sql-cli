package command

import (
	"os"
	"testing"
)

func TestGetMaxRowsFromEnv(t *testing.T) {
	// 保存原始环境变量，测试后恢复
	originalEnv := os.Getenv("SQL_CLI_MAX_ROWS")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("SQL_CLI_MAX_ROWS")
		} else {
			os.Setenv("SQL_CLI_MAX_ROWS", originalEnv)
		}
	}()

	cases := []struct {
		name string
		env  string
		want int
	}{
		{"default when not set", "", 1000},
		{"custom value", "500", 500},
		{"zero uses default", "0", 1000},
		{"negative uses default", "-100", 1000},
		{"invalid uses default", "abc", 1000},
		{"large value", "10000", 10000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env == "" {
				os.Unsetenv("SQL_CLI_MAX_ROWS")
			} else {
				os.Setenv("SQL_CLI_MAX_ROWS", tc.env)
			}

			got := getMaxRowsFromEnv()
			if got != tc.want {
				t.Errorf("getMaxRowsFromEnv() with env=%q: got %d, want %d", tc.env, got, tc.want)
			}
		})
	}
}
