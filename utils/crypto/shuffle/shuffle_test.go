package shuffle

import (
	"testing"
)

func Test_SHUFFLE(t *testing.T) {
	str := []byte("Hello World!12345")
	key := []byte("1234567890")

	encrypt_data := Encrypt(str, key)
	if string(encrypt_data) != "Hello World!09876" {
		t.Error(string(encrypt_data))
		t.Error("Hello World!09876")
		t.Error("fail!")
	}

	decrypt_data := Decrypt(encrypt_data, key)

	if string(str) != string(decrypt_data) {
		t.Error(string(str))
		t.Error(string(decrypt_data))
		t.Error("fail!")
	}
}
