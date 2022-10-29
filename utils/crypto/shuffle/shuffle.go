package shuffle

func unique(key []byte) []byte {
	if len(key) == 0 {
		return key
	}
	visited := make([]byte, 256)
	uniqkey := make([]byte, 0, len(key))
	for _, ch := range key {
		if visited[ch] == 0 {
			visited[ch] = 1
			uniqkey = append(uniqkey, ch)
		}
	}
	return uniqkey
}

func getTable(key []byte) []byte {
	table := make([]byte, 256)
	for i := 0; i < 256; i++ {
		table[i] = byte(i)
	}
	lenkey := len(key)
	for i, val := range key {
		if i < lenkey/2 {
			table[val], table[key[lenkey-i-1]] = table[key[lenkey-i-1]], table[val]
		}
	}
	return table
}

// Encrypt the data with key.
// data is the bytes to be encrypted.
// key is the encrypt key. It is the same as the decrypt key.
func Encrypt(data []byte, key []byte) []byte {
	if len(data) == 0 {
		return data
	}
	table := getTable(unique(key))
	res := make([]byte, len(data))
	for i, v := range data {
		res[i] = table[v]
	}
	return res
}

// Decrypt the data with key.
// data is the bytes to be decrypted.
// key is the decrypted key. It is the same as the encrypt key.
func Decrypt(data []byte, key []byte) []byte {
	return Encrypt(data, key)
}
