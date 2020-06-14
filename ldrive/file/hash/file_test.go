package file

import (
	"testing"
)

func TestHash(t *testing.T) {
	for i := 0; i < 2; i++ {
		hash, err := CalcCachedHash("_test_file.txt")
		if nil != err {
			t.Error("could not calculate hash", err)
		}
		if hash != "f20d9f2072bbeb6691c0f9c5099b01f3" {
			t.Error("caches don't match")
		}
	}
}
