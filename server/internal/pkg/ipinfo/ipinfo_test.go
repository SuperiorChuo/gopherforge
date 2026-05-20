package ipinfo

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestIPInfoPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("ipinfo.go")
	if err != nil {
		t.Fatalf("read ipinfo.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("ipinfo.go contains non-English source text")
	}
}

func TestPrivateIPInfoUsesEnglishLocation(t *testing.T) {
	client := NewIPInfoClient(time.Second, time.Minute)

	info, err := client.GetIPInfo("127.0.0.1")
	if err != nil {
		t.Fatalf("GetIPInfo(private): %v", err)
	}
	if info.Country != "Private Network" || info.City != "Private Network" {
		t.Fatalf("private location = country %q city %q, want Private Network", info.Country, info.City)
	}

	location := client.GetLocation("127.0.0.1")
	if location != "Private Network" {
		t.Fatalf("private location string = %q, want Private Network", location)
	}
}

func TestIPAPIRequestUsesEnglishLanguage(t *testing.T) {
	content, err := os.ReadFile("ipinfo.go")
	if err != nil {
		t.Fatalf("read ipinfo.go: %v", err)
	}

	if strings.Contains(string(content), "lang=zh-CN") {
		t.Fatal("ip-api request still asks for Chinese responses")
	}
}
