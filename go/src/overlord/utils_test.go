// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestGetFileSha1(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "TestGetFileSha1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString("TestGetFileSha1 string"); err != nil {
		t.Fatal(err)
	}

	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	sha1, err := GetFileSha1(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if sha1 != "2be19ce9f361fb1a6761998822f0b3ebbe151118" {
		t.Fatal("GetFileSha1 result error")
	}
}
