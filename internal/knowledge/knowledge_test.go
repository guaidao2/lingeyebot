package knowledge

import (
	"strings"
	"testing"
)

func TestStoreRoleIsolationAndUpdate(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.Create("worker", "web-recon", "Web 侦察方法", "先指纹，再目录。"); err != nil {
		t.Fatal(err)
	}
	docs, err := store.List("worker")
	if err != nil || len(docs) != 1 {
		t.Fatalf("list = %+v err=%v", docs, err)
	}
	doc, err := store.Get("worker", "web-recon")
	if err != nil || !strings.Contains(doc.Body, "指纹") {
		t.Fatalf("get = %+v err=%v", doc, err)
	}
	if _, err := store.Get("enumerator", "web-recon"); err == nil {
		t.Fatal("knowledge must be role-isolated")
	}
	if _, err := store.Update("worker", "web-recon", "更新描述", "新正文"); err != nil {
		t.Fatal(err)
	}
	doc, _ = store.Get("worker", "web-recon")
	if doc.Description != "更新描述" || !strings.Contains(doc.Body, "新正文") {
		t.Fatalf("updated = %+v", doc)
	}
	if !strings.Contains(store.Index("worker"), "web-recon: 更新描述") {
		t.Fatalf("index = %q", store.Index("worker"))
	}
}

func TestStoreRejectsTraversal(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.Create("worker", "../escape", "x", "body"); err == nil {
		t.Fatal("path traversal name must fail")
	}
	if _, err := store.Create("../worker", "x", "x", "body"); err == nil {
		t.Fatal("path traversal role must fail")
	}
}
