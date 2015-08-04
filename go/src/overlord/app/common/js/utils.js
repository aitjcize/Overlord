// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


function ReadBlobAsText(blob, callback) {
  var reader = new FileReader();
  reader.addEventListener("loadend", function() {
    callback(reader.result);
  });
  reader.readAsText(blob);
}
