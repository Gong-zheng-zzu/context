package tools

import (
	"context"
	"strings"
	"testing"
)

func TestDataManageRejectsArchiveWithoutDependencies(t *testing.T) {
	_, err := (&DataManageTool{}).Execute(context.Background(), "归档:张大爷的记录")
	if err == nil || !strings.Contains(err.Error(), "不执行归档") {
		t.Fatalf("archive must be explicitly rejected, got %v", err)
	}
}

func TestDataManageIsReadOnly(t *testing.T) {
	if !(&DataManageTool{}).IsReadOnly() {
		t.Fatal("data_manage must advertise read-only capability")
	}
}
