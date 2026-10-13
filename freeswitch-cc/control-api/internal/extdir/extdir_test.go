package extdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestWriteRemove(t *testing.T) {
	dir := t.TempDir()
	e := store.Extension{Ext: "2000", Password: "abcd1234", Label: `客服 <A> & "B"`}
	if err := Write(dir, e); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "2000.xml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `<user id="2000">`) || !strings.Contains(s, `value="abcd1234"`) {
		t.Errorf("xml missing fields:\n%s", s)
	}
	// 互拨必需：dial-string param 必须存在（缺失时 FS bridge user/ 报
	// No dial-string available，分机在线也打不通——08-08 全挂根因）。
	if !strings.Contains(s, DialStringParam) {
		t.Errorf("xml missing dial-string param:\n%s", s)
	}
	if strings.Contains(s, "<A>") || !strings.Contains(s, "&lt;A&gt;") {
		t.Errorf("label not escaped:\n%s", s)
	}

	if err := Remove(dir, "2000"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2000.xml")); !os.IsNotExist(err) {
		t.Error("file should be removed")
	}
	// 幂等：再删不报错
	if err := Remove(dir, "2000"); err != nil {
		t.Errorf("second remove: %v", err)
	}
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	exts := []store.Extension{
		{Ext: "2000", Password: "p2000"},
		{Ext: "3000", Password: "p3000", Label: "质检"},
	}
	if err := Sync(dir, exts); err != nil {
		t.Fatal(err)
	}
	for _, e := range exts {
		if _, err := os.Stat(filepath.Join(dir, e.Ext+".xml")); err != nil {
			t.Errorf("missing %s.xml", e.Ext)
		}
	}
}

func TestVoicemailParam(t *testing.T) {
	dir := t.TempDir()
	// 关闭语音信箱：不渲染 vm-password
	off := store.Extension{Ext: "2000", Password: "p2000"}
	if err := Write(dir, off); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "2000.xml"))
	if strings.Contains(string(raw), "vm-password") {
		t.Errorf("vm disabled should omit vm-password:\n%s", raw)
	}

	// 开启且指定密码
	on := store.Extension{Ext: "3000", Password: "p3000", VMEnabled: true, VMPassword: "1357"}
	if err := Write(dir, on); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "3000.xml"))
	if !strings.Contains(string(raw), `<param name="vm-password" value="1357"/>`) {
		t.Errorf("vm enabled should render vm-password:\n%s", raw)
	}

	// 开启但密码为空 → 跟随分机号
	blank := store.Extension{Ext: "4000", Password: "p4000", VMEnabled: true}
	if err := Write(dir, blank); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "4000.xml"))
	if !strings.Contains(string(raw), `<param name="vm-password" value="4000"/>`) {
		t.Errorf("blank vm password should default to ext:\n%s", raw)
	}
	if got := VMPasswordFor(blank); got != "4000" {
		t.Errorf("VMPasswordFor blank = %q, want 4000", got)
	}
}
