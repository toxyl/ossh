package utils

import (
	"strings"

	"github.com/dgryski/go-farm"
	"github.com/dgryski/go-spooky"
	"github.com/shawnohare/go-minhash"
	"github.com/toxyl/gutils"
)

func PayloadToHash(payload string) string {
	hash := func(words []string, size int) *minhash.MinHash {
		h := minhash.New(spooky.Hash64, farm.Hash64, size)
		for _, w := range words {
			h.Push(w)
		}
		return h
	}

	w := []string{}
	lines := strings.Split(payload, "\n")
	for _, line := range lines {
		w = append(w, strings.Split(line, " ")...)
	}

	signature := gutils.Uint64SliceToStringSlice(hash(w, 1).Signature())[0]
	return signature + "_" + gutils.StringToSha1(payload)
}
