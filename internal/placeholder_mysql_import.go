// Package placeholder ensures mysql driver stays in go.mod as direct dependency.
// This file will be removed once actual code imports the driver.
package placeholder

import _ "github.com/go-sql-driver/mysql"
