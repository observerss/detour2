package server

import (
	"reflect"
	"testing"
)

func TestParseDNSServers(t *testing.T) {
	got := ParseDNSServers("8.8.8.8,1.1.1.1:5353, [2001:4860:4860::8888]:53")
	want := []string{"8.8.8.8:53", "1.1.1.1:5353", "[2001:4860:4860::8888]:53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected DNS servers: got %#v want %#v", got, want)
	}
}
