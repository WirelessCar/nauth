package core

import (
	"os"
	"testing"

	approvals "github.com/approvals/go-approval-tests"
)

func TestMain(m *testing.M) {
	approvals.UseFolder("approvals")
	os.Exit(m.Run())
}
