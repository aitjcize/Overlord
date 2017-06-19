// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

function abbr(str, len) {
  if (str.length > len) {
    return str.substr(0, len - 3) + '...';
  }
  return str;
}

function ReadBlobAsText(blob, callback) {
  var reader = new FileReader();
  reader.addEventListener('loadend', function() {
    callback(reader.result);
  });
  reader.readAsText(blob);
}

function randomID() {
  return Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 10);
}

// Execute Overlord remote shell command
function getRemoteCmdOutput(mid, cmd) {
  var url = 'ws' + ((window.location.protocol == 'https:') ? 's' : '') +
            '://' + window.location.host + '/api/agent/shell/' + mid +
            '?command=' + cmd;
  var sock = new WebSocket(url);
  var deferred = $.Deferred();

  sock.onopen = function(event) {
    var blobs = [];
    sock.onmessage = function(msg) {
      if (msg.data instanceof Blob) {
        blobs.push(msg.data);
      }
    };
    sock.onclose = function(event) {
      var value = '';
      if (blobs.length == 0) {
        deferred.resolve('');
      }
      for (var i = 0; i < blobs.length; i++) {
        ReadBlobAsText(blobs[i], function(current) {
          return function(text) {
            value += text;
            if (current == blobs.length - 1) {
              deferred.resolve(value);
            }
          }
        }(i));
      }
    };
  };
  return deferred.promise();
}
