// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

function abbr(str, len) {
  if (str.length > len) {
    return str.substr(0, len - 3) + "...";
  }
  return str
}

function ReadBlobAsText(blob, callback) {
  var reader = new FileReader();
  reader.addEventListener("loadend", function() {
    callback(reader.result);
  });
  reader.readAsText(blob);
}

function randomID() {
  return Math.random().toString(36).replace(/[^a-z]+/g, "").substr(0, 10);
}
