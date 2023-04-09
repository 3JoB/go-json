// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json_test

import (
	"testing"

	"github.com/3JoB/go-json"
)

type basicLatin2xTag struct {
	V string `json:"$%-/"`
}

type basicLatin3xTag struct {
	V string `json:"0123456789"`
}

type basicLatin4xTag struct {
	V string `json:"ABCDEFGHIJKLMO"`
}

type basicLatin5xTag struct {
	V string `json:"PQRSTUVWXYZ_"`
}

type basicLatin6xTag struct {
	V string `json:"abcdefghijklmno"`
}

type basicLatin7xTag struct {
	V string `json:"pqrstuvwxyz"`
}

type miscPlaneTag struct {
	V string `json:"色は匂へど"`
}

type percentSlashTag struct {
	V string `json:"text/html%"` // https://golang.org/issue/2718
}

type punctuationTag struct {
	V string `json:"!#$%&()*+-./:<=>?@[]^_{|}~"` // https://golang.org/issue/3546
}

type dashTag struct {
	V string `json:"-,"`
}

type emptyTag struct {
	W string
}

type misnamedTag struct {
	X string `jsom:"Misnamed"`
}

type badFormatTag struct {
	Y string `:"BadFormat"`
}

type badCodeTag struct {
	Z string `json:" !\"#&'()*+,."`
}

type spaceTag struct {
	Q string `json:"With space"`
}

type unicodeTag struct {
	W string `json:"Ελλάδα"`
}

var structTagObjectKeyTests = []struct {
	raw   any
	value string
	key   string
}{
	{raw: basicLatin2xTag{V: "2x"}, value: "2x", key: "$%-/"},
	{raw: basicLatin3xTag{V: "3x"}, value: "3x", key: "0123456789"},
	{raw: basicLatin4xTag{V: "4x"}, value: "4x", key: "ABCDEFGHIJKLMO"},
	{raw: basicLatin5xTag{V: "5x"}, value: "5x", key: "PQRSTUVWXYZ_"},
	{raw: basicLatin6xTag{V: "6x"}, value: "6x", key: "abcdefghijklmno"},
	{raw: basicLatin7xTag{V: "7x"}, value: "7x", key: "pqrstuvwxyz"},
	{raw: miscPlaneTag{V: "いろはにほへと"}, value: "いろはにほへと", key: "色は匂へど"},
	{raw: dashTag{V: "foo"}, value: "foo", key: "-"},
	{raw: emptyTag{W: "Pour Moi"}, value: "Pour Moi", key: "W"},
	{raw: misnamedTag{X: "Animal Kingdom"}, value: "Animal Kingdom", key: "X"},
	{raw: badFormatTag{Y: "Orfevre"}, value: "Orfevre", key: "Y"},
	{raw: badCodeTag{Z: "Reliable Man"}, value: "Reliable Man", key: "Z"},
	{raw: percentSlashTag{V: "brut"}, value: "brut", key: "text/html%"},
	{raw: punctuationTag{V: "Union Rags"}, value: "Union Rags", key: "!#$%&()*+-./:<=>?@[]^_{|}~"},
	{raw: spaceTag{Q: "Perreddu"}, value: "Perreddu", key: "With space"},
	{raw: unicodeTag{W: "Loukanikos"}, value: "Loukanikos", key: "Ελλάδα"},
}

func TestStructTagObjectKey(t *testing.T) {
	for _, tt := range structTagObjectKeyTests {
		b, err := json.Marshal(tt.raw)
		if err != nil {
			t.Fatalf("Marshal(%#q) failed: %v", tt.raw, err)
		}
		var f any
		err = json.Unmarshal(b, &f)
		if err != nil {
			t.Fatalf("Unmarshal(%#q) failed: %v", b, err)
		}
		for i, v := range f.(map[string]any) {
			switch i {
			case tt.key:
				if s, ok := v.(string); !ok || s != tt.value {
					t.Fatalf("Unexpected value: %#q, want %v", s, tt.value)
				}
			default:
				t.Fatalf("Unexpected key: %#q, from %#q", i, b)
			}
		}
	}
}
