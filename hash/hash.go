/*
	hash pkg
	(c) 2014 Cergoo
	under terms of ISC license
*/

package hash

// FAQ6 hash
func HashFAQ6(str []byte) (h uint32) {
	for i := 0; i < len(str); i++ {
		h += uint32(str[i])
		h += h << 10
		h ^= h >> 6
	}
	h += h << 3
	h ^= h >> 11
	h += h << 15
	return
}

// Rot13 hash
func HashRot13(str []byte) (h uint32) {
	for i := range str {
		h += uint32(str[i])
		h -= (h << 13) | (h >> 19)
	}
	return
}

// Ly hash
func HashLy(str []byte) (h uint32) {
	for i := range str {
		h = (h * 1664525) + uint32(str[i]) + 1013904223
	}
	return
}

// Rs hash
func HashRs(str []byte) (h uint32) {
	const b = 378551
	var a uint32 = 63689
	for i := range str {
		h = h*a + uint32(str[i])
		a *= b
	}
	return
}
