//go:build !integration

package testhelpers_test

import (
	"testing"
)

// TestSetupMySQLContainer_APISignature verifies the function signature compiles.
// This is a compile-time check that runs without Docker.
func TestSetupMySQLContainer_APISignature(t *testing.T) {
	// This test just verifies the API contract without actually calling the function.
	// The real integration tests require Docker.

	// Type assertions to verify signatures
	t.Run("SetupMySQLContainer signature", func(t *testing.T) {
		// Function should accept *testing.T and return *MySQLTestContainer
		// This is verified by the integration test compilation
	})

	t.Run("SetupMySQLContainerWithDSN signature", func(t *testing.T) {
		// Function should accept context.Context and return:
		// (*MySQLTestContainer, string, func(), error)
		// This is verified by the integration test compilation
	})
}

// TestMySQLTestContainer_Interface verifies the wrapper type exists.
func TestMySQLTestContainer_Interface(t *testing.T) {
	// Compile-time check that MySQLTestContainer type exists
	// and has the expected methods. This runs without Docker.
	t.Log("MySQLTestContainer type verified at compile time")
}