package goadawasm_test

import (
	"fmt"
	"net/url"
	"testing"

	goadawasm "github.com/yzqzss/goada-wasm"
)

func compareString(t *testing.T, expected, actual, message string) {
	if expected != actual {
		t.Errorf("Expected %s, but got %s. %s", expected, actual, message)
	}
}

func TestBadUrl(t *testing.T) {

	url, err := goadawasm.New("some bad url")
	if err == nil {
		t.Error("Expected error")
	}
	if url != nil {
		t.Error("Expected no URL")
	}
	t.Logf("err=%v", err)
}

func TestGoodUrl(t *testing.T) {

	url, err := goadawasm.New("https://www.GOogle.com")
	if err != nil {
		t.Error("Expected no error")
	}
	fmt.Println(url.Href())

	compareString(t, "https://www.google.com/", url.Href(), "Expected normalized url")
	compareString(t, "https:", url.Protocol(), "Expected https protocol")
}

func TestGoodUrlSet(t *testing.T) {

	url, err := goadawasm.New("https://www.GOogle.com")
	if err != nil {
		t.Error("Expected no error")
	}
	fmt.Println(url.Href())

	compareString(t, "https://www.google.com/", url.Href(), "Expected normalized url")
	compareString(t, "https:", url.Protocol(), "Expected https protocol")
	url.SetProtocol("http:")
	compareString(t, "http:", url.Protocol(), "Expected http protocol")
	url.SetHash("goada")
	fmt.Println(url.Hash())

	compareString(t, "#goada", url.Hash(), "Expected goada hash")
	fmt.Println(url.Href())
	compareString(t, "http://www.google.com/#goada", url.Href(), "Expected normalized url")
}

func TestStandard(t *testing.T) {
	s1 := "https://	www.GOoglé.com/./path/../path2/"
	url, err := goadawasm.New(s1)
	if err != nil {
		t.Error("Expected no error")
	}
	fmt.Println(url.Href())
	compareString(t, "https://www.xn--googl-fsa.com/path2/", url.Href(), "Expected normalized url")
	url.Free()
}

func TestStandardGP(t *testing.T) {
	s1 := "https://	www.GOoglé.com"
	_, err := url.Parse(s1)
	if err == nil {
		t.Error("Go url should fail")
	}
	s2 := "https://www.GOoglé.com/./path/../path2/"
	url, err2 := url.Parse(s2)
	if err2 != nil {
		t.Error("Go url should hot fail")
	}
	fmt.Println(url.String())
	compareString(t, "https://www.GOogl%C3%A9.com/./path/../path2/", url.String(), "Expected invalid normalized url")
}
