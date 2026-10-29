package main

import (
	"strings"
	"testing"
)

func TestRDPPasswordInput(t *testing.T) {
	data, err := rdpPasswordInput([]string{"/v:127.0.0.1:13389", `/drive:work,C:\Folder With Spaces`, "+clipboard"}, "secret with spaces")
	if err != nil || string(data) != "/v:127.0.0.1:13389\n/drive:work,C:\\Folder With Spaces\n+clipboard\n/p:secret with spaces\n" {
		t.Fatal("secure argument serialization", err)
	}
	for _, pw := range []string{"", "bad\n/v:other", "bad\r", "bad\x00"} {
		if _, err := rdpPasswordInput(nil, pw); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if _, err := rdpPasswordInput([]string{"/title:bad\n/p:injected"}, "secret"); err == nil {
		t.Fatal("argument injection accepted")
	}
	clear(data)
	if strings.Trim(string(data), "\x00") != "" {
		t.Fatal("payload not cleared")
	}
}
